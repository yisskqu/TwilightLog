package content

import (
	"errors"
	"strings"
	"time"

	"github.com/yisskqu/TwilightLog/services/api/internal/media"
)

// Entity types stored in content_versions and used by reviews.
const (
	EntityArticleColumn = "article_column"
	EntityArticle       = "article"
	EntityPhotoColumn   = "photo_column"
	EntityAlbum         = "album"
	EntityPhoto         = "photo"
)

// Tag scopes keep the article and photography vocabularies independent.
const (
	ScopeArticle = "article"
	ScopePhoto   = "photo"
)

const (
	SortLatest = "latest"
	SortOldest = "oldest"

	DefaultPageSize = 12
	MaxPageSize     = 50
)

var (
	ErrNotFound     = errors.New("content not found")
	ErrInvalidInput = errors.New("invalid content query")
)

// Query is the shared filter set for every public list.
type Query struct {
	Page     int
	PageSize int
	Search   string
	Tags     []string
	DateFrom string // YYYY-MM-DD, inclusive
	DateTo   string // YYYY-MM-DD, inclusive
	Sort     string
}

// Normalize applies pagination and sort defaults.
func (q Query) Normalize() (Query, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = DefaultPageSize
	}
	if q.PageSize > MaxPageSize {
		q.PageSize = MaxPageSize
	}
	switch q.Sort {
	case "", SortLatest:
		q.Sort = SortLatest
	case SortOldest:
	default:
		return Query{}, ErrInvalidInput
	}
	q.Search = trimSpace(q.Search)
	q.DateFrom = trimSpace(q.DateFrom)
	q.DateTo = trimSpace(q.DateTo)
	for _, value := range []string{q.DateFrom, q.DateTo} {
		if value == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return Query{}, ErrInvalidInput
		}
	}
	if q.DateFrom != "" && q.DateTo != "" && q.DateFrom > q.DateTo {
		return Query{}, ErrInvalidInput
	}

	tags := make([]string, 0, len(q.Tags))
	seen := make(map[string]struct{}, len(q.Tags))
	for _, tag := range q.Tags {
		tag = trimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	q.Tags = tags
	return q, nil
}

// MediaAsset is a resolved media row ready for a public response.
type MediaAsset struct {
	ID       string
	Width    int
	Height   int
	Blurhash string
	Variants map[string]media.VariantInfo
	Exif     media.ExifInfo
}

// ColumnRef is the minimal parent reference shown in cards.
type ColumnRef struct {
	Slug string
	Name string
}

// ArticleColumn is a published article column.
type ArticleColumn struct {
	ID           string
	Slug         string
	Name         string
	Description  string
	Cover        *MediaAsset
	Tags         []string
	ArticleCount int
	UpdatedAt    time.Time
}

// Article is a published article with its public version snapshot.
type Article struct {
	ID           string
	Slug         string
	Title        string
	Summary      string
	BodyMarkdown string
	Column       ColumnRef
	Cover        *MediaAsset
	Tags         []string
	PublishedAt  time.Time
}

// PhotoColumn is a published photography column.
type PhotoColumn struct {
	ID          string
	Slug        string
	Name        string
	Description string
	Theme       string
	Cover       *MediaAsset
	Tags        []string
	AlbumCount  int
	UpdatedAt   time.Time
}

// Album is a published album. LatestPhotoAt is the newest public photo upload.
type Album struct {
	ID            string
	Slug          string
	Title         string
	Description   string
	Location      string
	ShotAt        string
	Cover         *MediaAsset
	Tags          []string
	PhotoCount    int
	LatestPhotoAt *time.Time
	Column        ColumnRef
	PublishedAt   time.Time
}

// Photo is a published photo with its approved display version.
type Photo struct {
	ID          string
	Title       string
	Description string
	Location    string
	TakenAt     string
	Album       ColumnRef
	Column      ColumnRef
	Tags        []string
	Media       MediaAsset
	PublishedAt time.Time
}

// Tag is one entry of the public tag vocabulary.
type Tag struct {
	Name string
}

// SearchHit is the normalized search result shared by all content types.
type SearchHit struct {
	Type        string
	ID          string
	Slug        string
	Title       string
	Summary     string
	Cover       *MediaAsset
	PublishedAt time.Time
	ColumnSlug  string
	ColumnName  string
	AlbumSlug   string
	AlbumTitle  string
}

// Recommendation points at one article or album selected for the home page.
type Recommendation struct {
	Type    string
	Article *Article
	Album   *Album
}

// RecommendationConfig is the published home page selection plus the counts
// configured by administrators.
type RecommendationConfig struct {
	Items    []Recommendation
	N1       int
	N2       int
	AutoFill bool
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
	return Page[T]{
		Items:      items,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

func trimSpace(value string) string {
	return strings.TrimSpace(value)
}

// Message strips the sentinel prefix so clients get a readable reason.
func Message(err error) string {
	prefix := ErrInvalidInput.Error() + ": "
	if strings.HasPrefix(err.Error(), prefix) {
		return strings.TrimPrefix(err.Error(), prefix)
	}
	return "请求参数不合法"
}
