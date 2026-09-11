// Package store 用 SQLite（纯 Go 驱动 modernc.org/sqlite）持久化手牌历史与动作流水，
// 支撑复盘与统计聚合。reviews 表为 M5 LLM 点评预留。
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// PlayerRecord 一手牌中单个参与者的记录（写入 hands.players_json）。
type PlayerRecord struct {
	Seat       int    `json:"seat"`
	Name       string `json:"name"`
	IsBot      bool   `json:"is_bot"`
	StartStack int    `json:"start_stack"`
	EndStack   int    `json:"end_stack"`
	Net        int    `json:"net"`
	Hole       string `json:"hole"` // 空格分隔，如 "As Kd"
}

// ActionRecord 一条动作流水（含盲注）。
type ActionRecord struct {
	Seq    int    `json:"seq"`
	Street string `json:"street"`
	Seat   int    `json:"seat"`
	Type   string `json:"type"`
	Amount int    `json:"amount"`
	To     int    `json:"to"`
}

// Hand 一手牌的完整记录。ListHands 返回时不含 Actions。
type Hand struct {
	ID         int64          `json:"id"`
	StartedAt  time.Time      `json:"started_at"`
	Button     int            `json:"button"`
	SmallBlind int            `json:"small_blind"`
	BigBlind   int            `json:"big_blind"`
	Seed       int64          `json:"seed"`
	Board      string         `json:"board"` // 空格分隔
	Players    []PlayerRecord `json:"players"`
	Actions    []ActionRecord `json:"actions,omitempty"`
}

// Stats 指定座位的聚合统计。
type Stats struct {
	Hands      int     `json:"hands"`
	VPIP       float64 `json:"vpip"` // 翻前自愿入池率 [0,1]
	PFR        float64 `json:"pfr"`  // 翻前加注率 [0,1]
	AF         float64 `json:"af"`   // 翻后 (bet+raise)/call；无跟注时为 0
	BetsRaises int     `json:"bets_raises"`
	Calls      int     `json:"calls"`
	NetProfit  int     `json:"net_profit"`
}

// DB 存储句柄。
type DB struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS hands (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	started_at  INTEGER NOT NULL,
	button      INTEGER NOT NULL,
	small_blind INTEGER NOT NULL,
	big_blind   INTEGER NOT NULL,
	seed        INTEGER NOT NULL,
	board       TEXT NOT NULL DEFAULT '',
	players_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS actions (
	hand_id INTEGER NOT NULL REFERENCES hands(id),
	seq     INTEGER NOT NULL,
	street  TEXT NOT NULL,
	seat    INTEGER NOT NULL,
	type    TEXT NOT NULL,
	amount  INTEGER NOT NULL DEFAULT 0,
	"to"    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (hand_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_actions_seat ON actions(seat);
CREATE TABLE IF NOT EXISTS reviews (
	hand_id    INTEGER PRIMARY KEY REFERENCES hands(id),
	content    TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
`

// Open 打开（必要时创建）数据库并建表。path 的父目录自动创建。
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: mkdir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// SQLite 单写者；限制一个连接避免 database is locked
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: schema: %w", err)
	}
	return &DB{db: db}, nil
}

// Close 关闭数据库。
func (d *DB) Close() error { return d.db.Close() }

// SaveReview 缓存一手的教练点评（覆盖旧值）。
func (d *DB) SaveReview(handID int64, content string) error {
	_, err := d.db.Exec(
		`INSERT INTO reviews (hand_id, content, created_at) VALUES (?,?,?)
		 ON CONFLICT(hand_id) DO UPDATE SET content=excluded.content, created_at=excluded.created_at`,
		handID, content, time.Now().UnixMilli())
	return err
}

// GetReview 返回一手的缓存点评；不存在返回 ("", nil)。
func (d *DB) GetReview(handID int64) (string, error) {
	var content string
	err := d.db.QueryRow(`SELECT content FROM reviews WHERE hand_id = ?`, handID).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return content, err
}

// SaveHand 在一个事务中写入 hands + actions，返回手牌 ID。
func (d *DB) SaveHand(h *Hand) (int64, error) {
	pj, err := json.Marshal(h.Players)
	if err != nil {
		return 0, fmt.Errorf("store: marshal players: %w", err)
	}
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(
		`INSERT INTO hands (started_at, button, small_blind, big_blind, seed, board, players_json)
		 VALUES (?,?,?,?,?,?,?)`,
		h.StartedAt.UnixMilli(), h.Button, h.SmallBlind, h.BigBlind, h.Seed, h.Board, string(pj))
	if err != nil {
		return 0, fmt.Errorf("store: insert hand: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, a := range h.Actions {
		if _, err := tx.Exec(
			`INSERT INTO actions (hand_id, seq, street, seat, type, amount, "to")
			 VALUES (?,?,?,?,?,?,?)`,
			id, i, a.Street, a.Seat, a.Type, a.Amount, a.To); err != nil {
			return 0, fmt.Errorf("store: insert action: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	h.ID = id
	return id, nil
}

func scanHand(row interface{ Scan(...any) error }) (*Hand, error) {
	var h Hand
	var startedMs int64
	var pj string
	if err := row.Scan(&h.ID, &startedMs, &h.Button, &h.SmallBlind, &h.BigBlind, &h.Seed, &h.Board, &pj); err != nil {
		return nil, err
	}
	h.StartedAt = time.UnixMilli(startedMs)
	if err := json.Unmarshal([]byte(pj), &h.Players); err != nil {
		return nil, fmt.Errorf("store: parse players_json: %w", err)
	}
	return &h, nil
}

const handCols = `id, started_at, button, small_blind, big_blind, seed, board, players_json`

// ListHands 按时间倒序返回手牌列表（不含动作流水）。
func (d *DB) ListHands(limit, offset int) ([]Hand, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.Query(
		`SELECT `+handCols+` FROM hands ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hand{}
	for rows.Next() {
		h, err := scanHand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

// ListHandsBefore 游标分页：返回 id < beforeID 的最新 limit 条（倒序）。
// beforeID <= 0 表示从最新开始。游标分页不受翻页期间新手牌入库的影响。
func (d *DB) ListHandsBefore(limit, beforeID int64) ([]Hand, error) {
	if limit <= 0 {
		limit = 50
	}
	if beforeID <= 0 {
		beforeID = 1<<62 - 1
	}
	rows, err := d.db.Query(
		`SELECT `+handCols+` FROM hands WHERE id < ? ORDER BY id DESC LIMIT ?`, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hand{}
	for rows.Next() {
		h, err := scanHand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

// GetHand 返回单手牌完整记录（含动作流水）。不存在返回 nil, nil。
func (d *DB) GetHand(id int64) (*Hand, error) {
	h, err := scanHand(d.db.QueryRow(`SELECT `+handCols+` FROM hands WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.db.Query(
		`SELECT seq, street, seat, type, amount, "to" FROM actions WHERE hand_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	h.Actions = []ActionRecord{}
	for rows.Next() {
		var a ActionRecord
		if err := rows.Scan(&a.Seq, &a.Street, &a.Seat, &a.Type, &a.Amount, &a.To); err != nil {
			return nil, err
		}
		h.Actions = append(h.Actions, a)
	}
	return h, rows.Err()
}

// Stats 聚合 seat 的统计数据（VPIP/PFR/AF/盈亏/手数）。
func (d *DB) Stats(seat int) (Stats, error) {
	var s Stats
	// 参与手数与盈亏：解析 players_json
	rows, err := d.db.Query(`SELECT players_json FROM hands`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var pj string
		if err := rows.Scan(&pj); err != nil {
			return s, err
		}
		var ps []PlayerRecord
		if err := json.Unmarshal([]byte(pj), &ps); err != nil {
			return s, fmt.Errorf("store: parse players_json: %w", err)
		}
		for _, p := range ps {
			if p.Seat == seat {
				s.Hands++
				s.NetProfit += p.Net
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return s, err
	}

	count := func(query string, args ...any) (int, error) {
		var n int
		err := d.db.QueryRow(query, args...).Scan(&n)
		return n, err
	}
	vpip, err := count(`SELECT COUNT(DISTINCT hand_id) FROM actions
		WHERE seat = ? AND street = 'preflop' AND type IN ('call','bet','raise')`, seat)
	if err != nil {
		return s, err
	}
	pfr, err := count(`SELECT COUNT(DISTINCT hand_id) FROM actions
		WHERE seat = ? AND street = 'preflop' AND type IN ('bet','raise')`, seat)
	if err != nil {
		return s, err
	}
	br, err := count(`SELECT COUNT(*) FROM actions
		WHERE seat = ? AND street != 'preflop' AND type IN ('bet','raise')`, seat)
	if err != nil {
		return s, err
	}
	calls, err := count(`SELECT COUNT(*) FROM actions
		WHERE seat = ? AND street != 'preflop' AND type = 'call'`, seat)
	if err != nil {
		return s, err
	}
	if s.Hands > 0 {
		s.VPIP = float64(vpip) / float64(s.Hands)
		s.PFR = float64(pfr) / float64(s.Hands)
	}
	s.BetsRaises = br
	s.Calls = calls
	if calls > 0 {
		s.AF = float64(br) / float64(calls)
	}
	return s, nil
}
