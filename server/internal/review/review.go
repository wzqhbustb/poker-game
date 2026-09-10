// Package review 生成一手牌的 LLM 教练点评：把手牌记录组装成中文 prompt，
// 调用 OpenAI 兼容的 chat completions API，结果由调用方缓存。
package review

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"pocker/server/internal/store"
)

// Config LLM 接入配置。APIKey 为空表示未启用。
type Config struct {
	BaseURL string // OpenAI 兼容 API 根，如 https://api.openai.com/v1
	APIKey  string
	Model   string
}

// Enabled 报告是否已配置 API key。
func (c Config) Enabled() bool { return c.APIKey != "" }

// buildPrompt 把手牌记录转成给教练模型的中文 prompt。
func buildPrompt(h *store.Hand) string {
	var b strings.Builder
	fmt.Fprintf(&b, "你是一位德州扑克教练，擅长用 GTO 思维（范围、底池赔率、最小防守频率、位置优势）分析牌局。\n")
	fmt.Fprintf(&b, "下面是一手 9 人桌无限注德州扑克（盲注 %d/%d）的完整记录，请分析人类玩家（You）的打法，\n", h.SmallBlind, h.BigBlind)
	b.WriteString("指出关键街的决策问题或亮点，并给出可操作的改进建议。用中文回答，控制在 300 字以内。\n\n")
	fmt.Fprintf(&b, "公共牌：%s\n", h.Board)
	b.WriteString("玩家（上帝视角，含所有底牌）：\n")
	for _, p := range h.Players {
		fmt.Fprintf(&b, "- 座位%d %s：底牌 %s，起始 %d，净盈亏 %+d\n", p.Seat, p.Name, p.Hole, p.StartStack, p.Net)
	}
	b.WriteString("行动流水：\n")
	street := ""
	for _, a := range h.Actions {
		if a.Street != street {
			street = a.Street
			fmt.Fprintf(&b, "[%s]\n", street)
		}
		name := fmt.Sprintf("座位%d", a.Seat)
		for _, p := range h.Players {
			if p.Seat == a.Seat {
				name = p.Name
			}
		}
		if a.Amount > 0 {
			fmt.Fprintf(&b, "  %s %s %d\n", name, a.Type, a.Amount)
		} else {
			fmt.Fprintf(&b, "  %s %s\n", name, a.Type)
		}
	}
	return b.String()
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate 为手牌 h 生成教练点评。调用方需先确认 Config.Enabled()。
func Generate(ctx context.Context, cfg Config, h *store.Hand) (string, error) {
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	body, err := json.Marshal(chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "user", Content: buildPrompt(h)},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("review: 请求 LLM 失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return "", fmt.Errorf("review: 解析响应失败: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("review: LLM 返回错误: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("review: LLM 未返回内容 (HTTP %d)", resp.StatusCode)
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}
