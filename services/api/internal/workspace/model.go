package workspace

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/yisskqu/TwilightLog/services/api/internal/content"
)

// Content types managed by the workspace. They mirror content.Entity* so the
// public API and the review workflow share one vocabulary.
const (
	TypeArticleColumn = content.EntityArticleColumn
	TypeArticle       = content.EntityArticle
	TypePhotoColumn   = content.EntityPhotoColumn
	TypeAlbum         = content.EntityAlbum
	TypePhoto         = content.EntityPhoto
)

// Status values exposed by the workspace list and the review queue.
const (
	StatusDraft     = "draft"
	StatusPending   = "pending"
	StatusRejected  = "rejected"
	StatusPublished = "published"
	StatusOffline   = "offline"
	StatusTrashed   = "trashed"
	StatusApproved  = "approved"
)

const (
	DefaultPageSize = 12
	MaxPageSize     = 50
	MaxTags         = 12
	MaxTagLength    = 40
)

var (
	ErrNotFound     = errors.New("workspace content not found")
	ErrForbidden    = errors.New("workspace access forbidden")
	ErrConflict     = errors.New("workspace state conflict")
	ErrInvalidInput = errors.New("invalid workspace input")
)

// TypeInfo describes one polymorphic content type. Table names are fixed
// constants chosen by Code, never taken from client input.
type TypeInfo struct {
	Code        string
	Table       string
	Scope       string
	ParentType  string
	ParentTable string
	TitleField  string
	HasSlug     bool
}

var typeInfos = map[string]TypeInfo{
	TypeArticleColumn: {Code: TypeArticleColumn, Table: "article_columns", Scope: content.ScopeArticle, TitleField: "name", HasSlug: true},
	TypeArticle:       {Code: TypeArticle, Table: "articles", Scope: content.ScopeArticle, ParentType: TypeArticleColumn, ParentTable: "article_columns", TitleField: "title", HasSlug: true},
	TypePhotoColumn:   {Code: TypePhotoColumn, Table: "photo_columns", Scope: content.ScopePhoto, TitleField: "name", HasSlug: true},
	TypeAlbum:         {Code: TypeAlbum, Table: "albums", Scope: content.ScopePhoto, ParentType: TypePhotoColumn, ParentTable: "photo_columns", TitleField: "title", HasSlug: true},
	TypePhoto:         {Code: TypePhoto, Table: "photos", Scope: content.ScopePhoto, ParentType: TypeAlbum, ParentTable: "albums", TitleField: "title"},
}

// TypeInfoFor validates a content type code.
func TypeInfoFor(code string) (TypeInfo, error) {
	info, ok := typeInfos[code]
	if !ok {
		return TypeInfo{}, fmt.Errorf("%w: 不支持的内容类型 %s", ErrInvalidInput, code)
	}
	return info, nil
}

// TypeCodes returns the supported types in menu order.
func TypeCodes() []string {
	return []string{TypeArticleColumn, TypeArticle, TypePhotoColumn, TypeAlbum, TypePhoto}
}

// Item is one row in a workspace or recycle-bin list.
type Item struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	Slug          string     `json:"slug"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	WorkingStatus string     `json:"workingStatus"`
	VersionID     string     `json:"versionId"`
	VersionNo     int        `json:"versionNo"`
	HasPublished  bool       `json:"hasPublished"`
	ParentID      string     `json:"parentId"`
	ParentTitle   string     `json:"parentTitle"`
	OwnerID       string     `json:"ownerId"`
	OwnerName     string     `json:"ownerName"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	TrashedAt     *time.Time `json:"trashedAt"`
	MediaCount    int        `json:"mediaCount"`
}

// Version is one content_versions row as seen by the editor.
type Version struct {
	ID          string         `json:"id"`
	Status      string         `json:"status"`
	VersionNo   int            `json:"versionNo"`
	Payload     map[string]any `json:"payload"`
	Comment     string         `json:"comment"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	SubmittedAt *time.Time     `json:"submittedAt"`
	PublishedAt *time.Time     `json:"publishedAt"`
}

// Detail carries the working version, the public version and entity metadata.
type Detail struct {
	Item
	Working   *Version `json:"working"`
	Published *Version `json:"published"`
}

// Page is the shared pagination envelope.
type Page[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

func newPage[T any](items []T, page, pageSize, total int) Page[T] {
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}
}

// ListQuery is shared by workspace lists and recycle bins.
type ListQuery struct {
	OwnerID  string
	Type     string
	Status   string
	Search   string
	Page     int
	PageSize int
}

// Normalize applies defaults and validates the status filter.
func (q ListQuery) Normalize() (ListQuery, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = DefaultPageSize
	}
	if q.PageSize > MaxPageSize {
		q.PageSize = MaxPageSize
	}
	q.Search = strings.TrimSpace(q.Search)
	switch q.Status {
	case "", "all", StatusDraft, StatusPending, StatusRejected, StatusPublished, StatusOffline, StatusTrashed:
		if q.Status == "all" {
			q.Status = ""
		}
	default:
		return ListQuery{}, fmt.Errorf("%w: 不支持的状态 %s", ErrInvalidInput, q.Status)
	}
	if q.Type != "" {
		if _, err := TypeInfoFor(q.Type); err != nil {
			return ListQuery{}, err
		}
	}
	return q, nil
}

// Stats is the per-type count block shown on the workspace home.
type Stats struct {
	Type      string `json:"type"`
	Total     int    `json:"total"`
	Draft     int    `json:"draft"`
	Pending   int    `json:"pending"`
	Rejected  int    `json:"rejected"`
	Published int    `json:"published"`
	Offline   int    `json:"offline"`
	Trashed   int    `json:"trashed"`
}

// SaveInput is the canonical editor payload for create and save.
type SaveInput struct {
	Slug     string         `json:"slug"`
	ParentID string         `json:"parentId"`
	Payload  map[string]any `json:"payload"`
}

// ReviewItem is one pending submission in the approval center.
type ReviewItem struct {
	VersionID    string         `json:"versionId"`
	Type         string         `json:"type"`
	EntityID     string         `json:"entityId"`
	Title        string         `json:"title"`
	VersionNo    int            `json:"versionNo"`
	Status       string         `json:"status"`
	AuthorID     string         `json:"authorId"`
	AuthorName   string         `json:"authorName"`
	SubmittedAt  *time.Time     `json:"submittedAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	HasPublished bool           `json:"hasPublished"`
	Payload      map[string]any `json:"payload"`
}

// ReviewDetail adds both versions and the tag snapshot for diffing.
type ReviewDetail struct {
	ReviewItem
	Published   map[string]any `json:"published"`
	Tags        []string       `json:"tags"`
	PublishedAt *time.Time     `json:"publishedAt"`
}

// Notification is one in-app message.
type Notification struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Severity  string     `json:"severity"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      string     `json:"link"`
	ReadAt    *time.Time `json:"readAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

// NotificationPage includes the unread badge count.
type NotificationPage struct {
	Items    []Notification `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Total    int            `json:"total"`
	Unread   int            `json:"unread"`
}

// TagUsage is one entry of the tag library.
type TagUsage struct {
	ID     string `json:"id"`
	Scope  string `json:"scope"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
	Usage  int    `json:"usage"`
}

// RecommendationItem is one configured home page slot.
type RecommendationItem struct {
	Position   int    `json:"position"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Title      string `json:"title"`
}

// RecommendationSettings is the administrator-managed home page configuration.
type RecommendationSettings struct {
	N1       int                  `json:"n1"`
	N2       int                  `json:"n2"`
	AutoFill bool                 `json:"autoFill"`
	Items    []RecommendationItem `json:"items"`
}

// Candidate is a public target that may be picked for the home page.
type Candidate struct {
	EntityType string    `json:"entityType"`
	EntityID   string    `json:"entityId"`
	Title      string    `json:"title"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// SystemInfo is the read-only deployment snapshot shown in the admin.
type SystemInfo struct {
	Version         string         `json:"version"`
	BuildStatus     string         `json:"buildStatus"`
	StorageUsage    int64          `json:"storageUsage"`
	JobStatus       map[string]any `json:"jobStatus"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	AppEnv          string         `json:"appEnv"`
	MaxArticleMB    int64          `json:"maxArticleMB"`
	MaxPhotoMB      int64          `json:"maxPhotoMB"`
	BatchLimit      int            `json:"batchLimit"`
	Processor       string         `json:"processor"`
	AccessTokenTTL  string         `json:"accessTokenTTL"`
	RefreshTokenTTL string         `json:"refreshTokenTTL"`
	CookieSecure    bool           `json:"cookieSecure"`
	Watermark       string         `json:"watermark"`
}

var markdownFence = regexp.MustCompile("(?s)```.*?```")
var markdownSymbols = regexp.MustCompile(`[#>*_\-\[\]()!` + "`" + `~|]`)
var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// NormalizePayload keeps only the fields that belong to the content type,
// validates required values and returns the canonical payload plus tag list.
func NormalizePayload(code string, raw map[string]any) (map[string]any, []string, error) {
	info, err := TypeInfoFor(code)
	if err != nil {
		return nil, nil, err
	}
	if raw == nil {
		raw = map[string]any{}
	}
	result := map[string]any{}
	switch code {
	case TypeArticleColumn:
		name, err := requiredString(raw, "name")
		if err != nil {
			return nil, nil, err
		}
		result["name"] = name
		result["description"] = optionalString(raw, "description")
		result["coverMediaId"] = optionalString(raw, "coverMediaId")
	case TypePhotoColumn:
		name, err := requiredString(raw, "name")
		if err != nil {
			return nil, nil, err
		}
		result["name"] = name
		result["description"] = optionalString(raw, "description")
		result["theme"] = optionalString(raw, "theme")
		result["coverMediaId"] = optionalString(raw, "coverMediaId")
	case TypeArticle:
		title, err := requiredString(raw, "title")
		if err != nil {
			return nil, nil, err
		}
		body := optionalString(raw, "bodyMarkdown")
		result["title"] = title
		result["summary"] = optionalString(raw, "summary")
		result["bodyMarkdown"] = body
		result["bodyIndex"] = BodyIndex(body)
		result["coverMediaId"] = optionalString(raw, "coverMediaId")
	case TypeAlbum:
		title, err := requiredString(raw, "title")
		if err != nil {
			return nil, nil, err
		}
		result["title"] = title
		result["description"] = optionalString(raw, "description")
		result["location"] = optionalString(raw, "location")
		shotAt := optionalString(raw, "shotAt")
		if shotAt != "" {
			if _, err := parseTimeValue(shotAt); err != nil {
				return nil, nil, fmt.Errorf("%w: 拍摄时间格式应为 YYYY-MM-DD 或 RFC3339", ErrInvalidInput)
			}
		}
		result["shotAt"] = shotAt
		result["coverMediaId"] = optionalString(raw, "coverMediaId")
	case TypePhoto:
		mediaID, err := requiredString(raw, "mediaId")
		if err != nil {
			return nil, nil, fmt.Errorf("%w: 照片必须先上传图片", ErrInvalidInput)
		}
		takenAt := optionalString(raw, "takenAt")
		if takenAt != "" {
			if _, err := parseTimeValue(takenAt); err != nil {
				return nil, nil, fmt.Errorf("%w: 拍摄时间格式应为 YYYY-MM-DD 或 RFC3339", ErrInvalidInput)
			}
		}
		result["mediaId"] = mediaID
		result["title"] = optionalString(raw, "title")
		result["description"] = optionalString(raw, "description")
		result["location"] = optionalString(raw, "location")
		result["takenAt"] = takenAt
	default:
		return nil, nil, fmt.Errorf("%w: 不支持的内容类型", ErrInvalidInput)
	}
	_ = info

	tags, err := normalizeTags(raw["tags"])
	if err != nil {
		return nil, nil, err
	}
	result["tags"] = tags
	return result, tags, nil
}

// TitleOf returns the display title for a canonical payload.
func TitleOf(code string, payload map[string]any) string {
	info, ok := typeInfos[code]
	if !ok {
		return ""
	}
	value, _ := payload[info.TitleField].(string)
	return value
}

// MediaIDs lists every media referenced by a canonical payload.
func MediaIDs(payload map[string]any) []string {
	var result []string
	for _, key := range []string{"coverMediaId", "mediaId"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}

// BodyIndex strips Markdown fences and punctuation for the public search index.
func BodyIndex(body string) string {
	cleaned := markdownFence.ReplaceAllString(body, " ")
	cleaned = markdownSymbols.ReplaceAllString(cleaned, " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if len(cleaned) > 20000 {
		cleaned = cleaned[:20000]
	}
	return cleaned
}

// Slugify produces an ASCII slug or an empty string when the title has no ASCII.
func Slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func requiredString(raw map[string]any, key string) (string, error) {
	value, _ := raw[key].(string)
	value = strings.TrimSpace(value)
	if value == "" {
		labels := map[string]string{"name": "名称", "title": "标题", "mediaId": "图片"}
		return "", fmt.Errorf("%w: %s不能为空", ErrInvalidInput, labels[key])
	}
	return value, nil
}

func optionalString(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return strings.TrimSpace(value)
}

func normalizeTags(value any) ([]string, error) {
	var items []string
	switch typed := value.(type) {
	case []string:
		items = typed
	case []any:
		for _, entry := range typed {
			if text, ok := entry.(string); ok {
				items = append(items, text)
			}
		}
	case nil:
		return []string{}, nil
	default:
		return nil, fmt.Errorf("%w: 标签格式不正确", ErrInvalidInput)
	}
	result := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if len([]rune(item)) > MaxTagLength {
			return nil, fmt.Errorf("%w: 单个标签不能超过 %d 个字符", ErrInvalidInput, MaxTagLength)
		}
		if strings.Contains(item, ",") {
			return nil, fmt.Errorf("%w: 标签不能包含逗号", ErrInvalidInput)
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	if len(result) > MaxTags {
		return nil, fmt.Errorf("%w: 最多 %d 个标签", ErrInvalidInput, MaxTags)
	}
	return result, nil
}

func parseTimeValue(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("invalid time")
}

// Message strips the sentinel prefix so clients get a readable reason.
func Message(err error) string {
	for _, sentinel := range []error{ErrInvalidInput, ErrConflict, ErrForbidden} {
		prefix := sentinel.Error() + ": "
		if strings.HasPrefix(err.Error(), prefix) {
			return strings.TrimPrefix(err.Error(), prefix)
		}
	}
	return "操作无法完成"
}
