package content

import (
	"context"
	"fmt"
)

// Service applies public query defaults and validates filter values.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// ListArticleColumns lists published article columns.
func (s *Service) ListArticleColumns(ctx context.Context, q Query) (Page[ArticleColumn], error) {
	q, err := q.Normalize()
	if err != nil {
		return Page[ArticleColumn]{}, err
	}
	return s.repo.ListArticleColumns(ctx, q)
}

// ListArticles lists published articles; an empty column slug means every column.
func (s *Service) ListArticles(ctx context.Context, columnSlug string, q Query) (Page[Article], error) {
	q, err := q.Normalize()
	if err != nil {
		return Page[Article]{}, err
	}
	return s.repo.ListArticles(ctx, trimSpace(columnSlug), q)
}

// GetArticle returns one published article.
func (s *Service) GetArticle(ctx context.Context, slug string) (Article, error) {
	if trimSpace(slug) == "" {
		return Article{}, ErrNotFound
	}
	return s.repo.GetArticle(ctx, slug)
}

// GetArticleColumn returns one published article column.
func (s *Service) GetArticleColumn(ctx context.Context, slug string) (ArticleColumn, error) {
	if trimSpace(slug) == "" {
		return ArticleColumn{}, ErrNotFound
	}
	return s.repo.GetArticleColumn(ctx, slug)
}

// GetPhotoColumn returns one published photography column.
func (s *Service) GetPhotoColumn(ctx context.Context, slug string) (PhotoColumn, error) {
	if trimSpace(slug) == "" {
		return PhotoColumn{}, ErrNotFound
	}
	return s.repo.GetPhotoColumn(ctx, slug)
}

// ListPhotoColumns lists published photography columns.
func (s *Service) ListPhotoColumns(ctx context.Context, theme string, q Query) (Page[PhotoColumn], error) {
	q, err := q.Normalize()
	if err != nil {
		return Page[PhotoColumn]{}, err
	}
	return s.repo.ListPhotoColumns(ctx, trimSpace(theme), q)
}

// ListAlbums lists published albums, optionally scoped to one column slug.
func (s *Service) ListAlbums(ctx context.Context, columnSlug string, q Query) (Page[Album], error) {
	q, err := q.Normalize()
	if err != nil {
		return Page[Album]{}, err
	}
	return s.repo.ListAlbums(ctx, trimSpace(columnSlug), q)
}

// GetAlbum returns one published album.
func (s *Service) GetAlbum(ctx context.Context, slug string) (Album, error) {
	if trimSpace(slug) == "" {
		return Album{}, ErrNotFound
	}
	return s.repo.GetAlbum(ctx, slug)
}

// ListPhotos lists published photos, optionally scoped to an album or column.
func (s *Service) ListPhotos(ctx context.Context, albumSlug, columnSlug string, q Query) (Page[Photo], error) {
	q, err := q.Normalize()
	if err != nil {
		return Page[Photo]{}, err
	}
	return s.repo.ListPhotos(ctx, trimSpace(albumSlug), trimSpace(columnSlug), q)
}

// GetPhoto returns one published photo.
func (s *Service) GetPhoto(ctx context.Context, id string) (Photo, error) {
	if trimSpace(id) == "" {
		return Photo{}, ErrNotFound
	}
	return s.repo.GetPhoto(ctx, id)
}

// ListTags returns the public tag vocabulary for one scope.
func (s *Service) ListTags(ctx context.Context, scope string) ([]Tag, error) {
	if scope != ScopeArticle && scope != ScopePhoto {
		return nil, ErrInvalidInput
	}
	return s.repo.ListTags(ctx, scope)
}

// Recommendations returns the configured home page selection.
func (s *Service) Recommendations(ctx context.Context, limit int) ([]Recommendation, error) {
	if limit <= 0 {
		limit = 6
	}
	if limit > 12 {
		limit = 12
	}
	return s.repo.ListRecommendations(ctx, limit)
}

var searchTypes = map[string]struct{}{
	EntityArticleColumn: {},
	EntityArticle:       {},
	EntityPhotoColumn:   {},
	EntityAlbum:         {},
	EntityPhoto:         {},
}

// Search searches every content type, or only the requested ones.
func (s *Service) Search(ctx context.Context, q Query, types []string) (Page[SearchHit], error) {
	q, err := q.Normalize()
	if err != nil {
		return Page[SearchHit]{}, err
	}
	if q.Search == "" {
		return Page[SearchHit]{}, fmt.Errorf("%w: 搜索关键词不能为空", ErrInvalidInput)
	}
	for _, kind := range types {
		if _, ok := searchTypes[kind]; !ok {
			return Page[SearchHit]{}, fmt.Errorf("%w: 不支持的内容类型 %s", ErrInvalidInput, kind)
		}
	}
	return s.repo.Search(ctx, q, types)
}
