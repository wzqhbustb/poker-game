// pokerd 德州扑克练习平台后端：WebSocket 牌桌服务 + 复盘/统计 REST API。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"pocker/server/internal/proto"
	"pocker/server/internal/review"
	"pocker/server/internal/store"
	"pocker/server/internal/table"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	dbPath := flag.String("db", "data/poker.db", "SQLite 数据库路径")
	buyin := flag.Int("buyin", 200, "买入筹码（100bb）")
	timeout := flag.Duration("timeout", 30*time.Second, "人类行动超时（超时自动 check/fold）")
	fast := flag.Bool("fast", false, "bot 不 sleep（测试用）")
	seed := flag.Int64("seed", 0, "牌桌随机种子，0 表示随机")
	webDir := flag.String("web", "web/dist", "前端静态文件目录")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	tbl := table.New(table.Config{
		BuyIn:         *buyin,
		ActionTimeout: *timeout,
		Fast:          *fast,
		Seed:          *seed,
	}, st)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go tbl.Run(ctx)

	reviewCfg := review.Config{
		BaseURL: os.Getenv("POKER_LLM_BASE_URL"),
		APIKey:  os.Getenv("POKER_LLM_API_KEY"),
		Model:   os.Getenv("POKER_LLM_MODEL"),
	}
	if reviewCfg.Enabled() && reviewCfg.Model == "" {
		reviewCfg.Model = "gpt-4o-mini"
	}
	if !reviewCfg.Enabled() {
		log.Printf("POKER_LLM_API_KEY 未设置，教练点评功能停用")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", serveWS(tbl))
	mux.HandleFunc("GET /api/hands", listHands(st))
	mux.HandleFunc("GET /api/hands/{id}", getHand(st))
	mux.HandleFunc("GET /api/stats", getStats(st))
	mux.HandleFunc("GET /api/review/{id}", getReview(st, reviewCfg))
	mux.HandleFunc("POST /api/review/{id}", postReview(st, reviewCfg))
	if info, err := os.Stat(*webDir); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(*webDir)))
		log.Printf("serving web from %s", *webDir)
	}

	srv := &http.Server{Addr: *addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	log.Printf("pokerd listening on %s, db=%s buyin=%d timeout=%s fast=%v",
		*addr, *dbPath, *buyin, *timeout, *fast)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http: %v", err)
	}
}

// serveWS 升级 WebSocket 并桥接到桌子：读循环解析客户端消息，写协程推送桌子消息。
func serveWS(tbl *table.Table) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // 本地练习工具，放开来源校验
		})
		if err != nil {
			log.Printf("ws accept: %v", err)
			return
		}
		defer conn.CloseNow()

		client := &table.Client{Name: "You", Send: make(chan any, 256)}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		defer tbl.Detach(client)

		// 写协程：桌子 → 浏览器
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case msg := <-client.Send:
					data, err := proto.Encode(msg)
					if err != nil {
						log.Printf("ws encode: %v", err)
						continue
					}
					wctx, wc := context.WithTimeout(ctx, 10*time.Second)
					err = conn.Write(wctx, websocket.MessageText, data)
					wc()
					if err != nil {
						cancel()
						return
					}
				}
			}
		}()

		// 读循环：浏览器 → 桌子
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			m, err := proto.DecodeClient(data)
			if err != nil {
				select {
				case client.Send <- proto.ErrorMsg{Type: proto.SError, Message: err.Error()}:
				default:
				}
				continue
			}
			switch m := m.(type) {
			case *proto.Hello:
				if m.Name != "" {
					client.Name = m.Name
				}
				tbl.Attach(client)
			case *proto.ActionMsg:
				tbl.SubmitAction(client, m.Action, m.Amount)
			case *proto.RebuyMsg:
				tbl.Rebuy(client)
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func listHands(st *store.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		hands, err := st.ListHands(limit, offset)
		writeJSON(w, hands, err)
	}
}

func getHand(st *store.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		h, err := st.GetHand(id)
		if err == nil && h == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, h, err)
	}
}

func getStats(st *store.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := st.Stats(table.HumanSeat)
		writeJSON(w, stats, err)
	}
}

// getReview 返回一手的缓存点评。enabled 反映是否已配置 LLM。
func getReview(st *store.DB, cfg review.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		content, err := st.GetReview(id)
		writeJSON(w, map[string]any{
			"content": content, "enabled": cfg.Enabled(),
		}, err)
	}
}

// postReview 生成（或重生成）一手的教练点评并缓存。
func postReview(st *store.DB, cfg review.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.Enabled() {
			http.Error(w, "未配置 POKER_LLM_API_KEY，教练点评不可用", http.StatusServiceUnavailable)
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		h, err := st.GetHand(id)
		if err != nil || h == nil {
			http.Error(w, "hand not found", http.StatusNotFound)
			return
		}
		content, err := review.Generate(r.Context(), cfg, h)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if err := st.SaveReview(id, content); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"content": content, "enabled": true}, nil)
	}
}
