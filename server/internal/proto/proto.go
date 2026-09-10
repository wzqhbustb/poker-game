// Package proto 定义 WebSocket JSON 消息协议（服务端权威）。
// 所有消息以 type 字段区分；牌面用 "As" 风格短字符串表示。
package proto

import (
	"encoding/json"
	"fmt"
)

// 客户端 → 服务端消息类型。
const (
	CHello  = "hello"
	CAction = "action"
	CRebuy  = "rebuy"
)

// 服务端 → 客户端消息类型。
const (
	SWelcome       = "welcome"
	SState         = "state"
	SActionRequest = "action_request"
	SHandEvent     = "hand_event"
	SHandEnd       = "hand_end"
	SError         = "error"
)

// hand_event 的 kind 取值。
const (
	EvHandStart = "hand_start"
	EvAction    = "action"
	EvStreet    = "street"
)

// ---------------------------------------------------------------- client → server

// Hello 入座请求：建立连接后第一个消息。
type Hello struct {
	Type string `json:"type"` // "hello"
	Name string `json:"name"`
}

// ActionMsg 人类玩家动作。Amount 仅 raise 使用，为 raise-to 本街总下注额。
type ActionMsg struct {
	Type   string `json:"type"` // "action"
	Action string `json:"action"` // fold|check|call|raise
	Amount int    `json:"amount,omitempty"`
}

// RebuyMsg 补码请求：把座位筹码补充到买入。
type RebuyMsg struct {
	Type string `json:"type"` // "rebuy"
}

// DecodeClient 解析客户端消息。
func DecodeClient(data []byte) (any, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("proto: bad json: %w", err)
	}
	var m any
	switch head.Type {
	case CHello:
		m = &Hello{}
	case CAction:
		m = &ActionMsg{}
	case CRebuy:
		m = &RebuyMsg{}
	default:
		return nil, fmt.Errorf("proto: unknown client message type %q", head.Type)
	}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("proto: bad %s message: %w", head.Type, err)
	}
	return m, nil
}

// ---------------------------------------------------------------- server → client

// TableConfig 牌桌配置，随 welcome 下发。
type TableConfig struct {
	Seats           int   `json:"seats"`
	SmallBlind      int   `json:"small_blind"`
	BigBlind        int   `json:"big_blind"`
	BuyIn           int   `json:"buyin"`
	ActionTimeoutMs int64 `json:"action_timeout_ms"`
}

// Welcome 入座成功。
type Welcome struct {
	Type   string      `json:"type"` // "welcome"
	Seat   int         `json:"seat"`
	Config TableConfig `json:"config"`
}

// SeatState 单个座位的按接收者裁剪后的快照。
type SeatState struct {
	Seat     int      `json:"seat"`
	Name     string   `json:"name"`
	StyleTag string   `json:"style_tag,omitempty"`
	IsBot    bool     `json:"is_bot"`
	Stack    int      `json:"stack"`
	Bet      int      `json:"bet"`
	Folded   bool     `json:"folded"`
	AllIn    bool     `json:"allin"`
	Out      bool     `json:"out"`
	ToAct    bool     `json:"to_act"`
	Thinking bool     `json:"thinking"`
	Hole     []string `json:"hole,omitempty"` // 仅接收者本人；摊牌后含未弃牌者
}

// State 牌桌快照，只发给人类客户端（已按其视角裁剪）。
type State struct {
	Type   string      `json:"type"` // "state"
	HandNo int64       `json:"hand_no"`
	InHand bool        `json:"in_hand"`
	Seats  []SeatState `json:"seats"`
	Board  []string    `json:"board"`
	Pot    int         `json:"pot"`
	Street string      `json:"street"`
	Button int         `json:"button"`
	You    int         `json:"you"`
	Ts     int64       `json:"ts"` // 服务器毫秒时间戳
}

// LegalInfo 当前合法动作集合。
type LegalInfo struct {
	CanFold    bool `json:"can_fold"`
	CanCheck   bool `json:"can_check"`
	CallAmount int  `json:"call_amount"`
	CanRaise   bool `json:"can_raise"`
	MinRaiseTo int  `json:"min_raise_to"`
	MaxRaiseTo int  `json:"max_raise_to"`
}

// ActionRequest 轮到人类行动，Deadline 为 unix 毫秒；超时服务端自动 check/fold。
type ActionRequest struct {
	Type       string    `json:"type"` // "action_request"
	Deadline   int64     `json:"deadline"`
	Legal      LegalInfo `json:"legal"`
	Pot        int       `json:"pot"`
	CurrentBet int       `json:"current_bet"`
}

// HandEvent 手牌内增量事件（动作/发街/手牌开始），供前端动画。
type HandEvent struct {
	Type   string   `json:"type"` // "hand_event"
	HandNo int64    `json:"hand_no"`
	Kind   string   `json:"kind"` // hand_start|action|street
	Street string   `json:"street,omitempty"`
	Seat   int      `json:"seat,omitempty"`   // kind=action 时的行动者；hand_start 时为按钮位
	Action string   `json:"action,omitempty"` // fold|check|call|bet|raise|blind
	Amount int      `json:"amount,omitempty"` // 本次投入增量
	To     int      `json:"to,omitempty"`     // 动作后本街总下注
	Board  []string `json:"board,omitempty"`
	Pot    int      `json:"pot"`
}

// ResultInfo 单个座位结算。
type ResultInfo struct {
	Seat     int  `json:"seat"`
	Won      int  `json:"won"`
	Net      int  `json:"net"`
	Showdown bool `json:"showdown"`
}

// PotInfo 主池/边池分配。
type PotInfo struct {
	Amount  int   `json:"amount"`
	Winners []int `json:"winners"`
}

// RevealedHole 摊牌揭示的底牌。
type RevealedHole struct {
	Seat int      `json:"seat"`
	Hole []string `json:"hole"`
}

// HandEnd 手牌结束。HandID 为存储层 ID，复盘用。
type HandEnd struct {
	Type          string         `json:"type"` // "hand_end"
	HandNo        int64          `json:"hand_no"`
	HandID        int64          `json:"hand_id"`
	Results       []ResultInfo   `json:"results"`
	Pots          []PotInfo      `json:"pots"`
	RevealedHoles []RevealedHole `json:"revealed_holes,omitempty"`
}

// ErrorMsg 服务端错误提示。
type ErrorMsg struct {
	Type    string `json:"type"` // "error"
	Message string `json:"message"`
}

// Encode 序列化服务端消息。
func Encode(v any) ([]byte, error) { return json.Marshal(v) }
