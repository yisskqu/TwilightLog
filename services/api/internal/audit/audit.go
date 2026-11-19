package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
}

type Entry struct {
	ID         int64          `json:"id"`
	ActorID    *string        `json:"actorId"`
	ActorName  string         `json:"actorName"`
	Action     string         `json:"action"`
	TargetType string         `json:"targetType"`
	TargetID   string         `json:"targetId"`
	Result     string         `json:"result"`
	IP         string         `json:"ip"`
	UserAgent  string         `json:"userAgent"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"createdAt"`
}

type Page struct {
	Items    []Entry `json:"items"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Total    int64   `json:"total"`
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Log(ctx context.Context, actorID *string, action, targetType, targetID, result, ip, userAgent string, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return
	}
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, ip, user_agent, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
	`, actorID, action, targetType, targetID, result, truncate(ip, 64), truncate(userAgent, 256), string(payload))
}

// Filter narrows the audit page without allowing edits.
type Filter struct {
	Action     string
	Actor      string
	Result     string
	TargetType string
	From       string
	To         string
}

func (s *Service) List(ctx context.Context, filter Filter, page, pageSize int) (Page, error) {
	page, pageSize = normalizePage(page, pageSize)

	var args []any
	where := "WHERE 1 = 1"
	add := func(value any, fragment string) {
		args = append(args, value)
		where += fmt.Sprintf(fragment, len(args))
	}
	if filter.Action != "" {
		add(filter.Action, " AND a.action = $%d")
	}
	if filter.Result != "" {
		add(filter.Result, " AND a.result = $%d")
	}
	if filter.TargetType != "" {
		add(filter.TargetType, " AND a.target_type = $%d")
	}
	if filter.Actor != "" {
		add("%"+filter.Actor+"%", " AND u.username ILIKE $%d")
	}
	if filter.From != "" {
		add(filter.From, " AND a.created_at >= $%d::date")
	}
	if filter.To != "" {
		add(filter.To, " AND a.created_at < ($%d::date + interval '1 day')")
	}

	var total int64
	countSQL := `SELECT count(*) FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id ` + where
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count audit logs: %w", err)
	}

	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.actor_id::text, COALESCE(u.username, ''), a.action, a.target_type,
		       a.target_id, a.result, a.ip, a.user_agent, a.metadata, a.created_at
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_id
		`+where+`
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return Page{}, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	items := make([]Entry, 0, pageSize)
	for rows.Next() {
		var item Entry
		var metadata []byte
		if err := rows.Scan(
			&item.ID,
			&item.ActorID,
			&item.ActorName,
			&item.Action,
			&item.TargetType,
			&item.TargetID,
			&item.Result,
			&item.IP,
			&item.UserAgent,
			&metadata,
			&item.CreatedAt,
		); err != nil {
			return Page{}, fmt.Errorf("scan audit log: %w", err)
		}
		item.Metadata = map[string]any{}
		_ = json.Unmarshal(metadata, &item.Metadata)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate audit logs: %w", err)
	}

	return Page{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 12
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
