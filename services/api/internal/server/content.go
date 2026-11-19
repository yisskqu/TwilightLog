package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yisskqu/TwilightLog/services/api/internal/content"
	"github.com/yisskqu/TwilightLog/services/api/internal/media"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/httpx"
)

type assetVariantResponse struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int64  `json:"size"`
}

type assetResponse struct {
	MediaID  string                 `json:"mediaId"`
	Key      string                 `json:"key"`
	Width    int                    `json:"width"`
	Height   int                    `json:"height"`
	Blurhash string                 `json:"blurhash"`
	Variants []assetVariantResponse `json:"variants"`
}

type columnRefResponse struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type articleColumnResponse struct {
	ID           string         `json:"id"`
	Slug         string         `json:"slug"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Cover        *assetResponse `json:"cover"`
	Tags         []string       `json:"tags"`
	ArticleCount int            `json:"articleCount"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

type articleResponse struct {
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	Column      columnRefResponse `json:"column"`
	Cover       *assetResponse    `json:"cover"`
	Tags        []string          `json:"tags"`
	PublishedAt time.Time         `json:"publishedAt"`
}

type articleDetailResponse struct {
	articleResponse
	BodyMarkdown string `json:"bodyMarkdown"`
}

type photoColumnResponse struct {
	ID          string         `json:"id"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Theme       string         `json:"theme"`
	Cover       *assetResponse `json:"cover"`
	Tags        []string       `json:"tags"`
	AlbumCount  int            `json:"albumCount"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type albumResponse struct {
	ID            string            `json:"id"`
	Slug          string            `json:"slug"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Location      string            `json:"location"`
	ShotAt        string            `json:"shotAt"`
	Column        columnRefResponse `json:"column"`
	Cover         *assetResponse    `json:"cover"`
	Tags          []string          `json:"tags"`
	PhotoCount    int               `json:"photoCount"`
	LatestPhotoAt *time.Time        `json:"latestPhotoAt"`
	PublishedAt   time.Time         `json:"publishedAt"`
}

type photoResponse struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Location    string            `json:"location"`
	TakenAt     string            `json:"takenAt"`
	Album       columnRefResponse `json:"album"`
	Column      columnRefResponse `json:"column"`
	Tags        []string          `json:"tags"`
	Media       assetResponse     `json:"media"`
	PublishedAt time.Time         `json:"publishedAt"`
}

type photoDetailResponse struct {
	photoResponse
	Exif media.ExifInfo `json:"exif"`
}

type searchHitResponse struct {
	Type        string            `json:"type"`
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	Cover       *assetResponse    `json:"cover"`
	PublishedAt time.Time         `json:"publishedAt"`
	Column      columnRefResponse `json:"column"`
	Album       columnRefResponse `json:"album"`
}

type tagResponse struct {
	Name string `json:"name"`
}

type recommendationResponse struct {
	Type    string           `json:"type"`
	Article *articleResponse `json:"article,omitempty"`
	Album   *albumResponse   `json:"album,omitempty"`
}

type articleColumnListResponse = content.Page[articleColumnResponse]
type photoColumnListResponse = content.Page[photoColumnResponse]
type photoListResponse = content.Page[photoResponse]
type searchListResponse = content.Page[searchHitResponse]

type columnArticlesResponse struct {
	Column     columnRefResponse `json:"column"`
	Items      []articleResponse `json:"items"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	Total      int               `json:"total"`
	TotalPages int               `json:"totalPages"`
}

type columnAlbumsResponse struct {
	Column     columnRefResponse `json:"column"`
	Items      []albumResponse   `json:"items"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	Total      int               `json:"total"`
	TotalPages int               `json:"totalPages"`
}

type recommendationsResponse struct {
	Items    []recommendationResponse `json:"items"`
	N1       int                      `json:"n1"`
	N2       int                      `json:"n2"`
	AutoFill bool                     `json:"autoFill"`
}

func recommendationsHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		config, err := svc.Recommendations(r.Context(), limit)
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		result := recommendationsResponse{
			Items:    make([]recommendationResponse, 0, len(config.Items)),
			N1:       config.N1,
			N2:       config.N2,
			AutoFill: config.AutoFill,
		}
		for _, item := range config.Items {
			entry := recommendationResponse{Type: item.Type}
			if item.Article != nil {
				article := newArticleResponse(mediaSvc, *item.Article)
				entry.Article = &article
			}
			if item.Album != nil {
				album := newAlbumResponse(mediaSvc, *item.Album)
				entry.Album = &album
			}
			result.Items = append(result.Items, entry)
		}
		httpx.WriteJSONETag(w, r, result)
	}
}

func listArticleColumnsHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := svc.ListArticleColumns(r.Context(), contentQuery(r))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		items := make([]articleColumnResponse, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, newArticleColumnResponse(mediaSvc, item))
		}
		httpx.WriteJSONETag(w, r, articleColumnListResponse{
			Items: items, Page: page.Page, PageSize: page.PageSize,
			Total: page.Total, TotalPages: page.TotalPages,
		})
	}
}

func listColumnArticlesHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		column, err := svc.GetArticleColumn(r.Context(), slug)
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		page, err := svc.ListArticles(r.Context(), slug, contentQuery(r))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		items := make([]articleResponse, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, newArticleResponse(mediaSvc, item))
		}
		httpx.WriteJSONETag(w, r, columnArticlesResponse{
			Column: columnRefResponse{Slug: column.Slug, Name: column.Name},
			Items:  items, Page: page.Page, PageSize: page.PageSize,
			Total: page.Total, TotalPages: page.TotalPages,
		})
	}
}

func getArticleHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		article, err := svc.GetArticle(r.Context(), chi.URLParam(r, "slug"))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		httpx.WriteJSONETag(w, r, articleDetailResponse{
			articleResponse: newArticleResponse(mediaSvc, article),
			BodyMarkdown:    article.BodyMarkdown,
		})
	}
}

func listPhotoColumnsHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := svc.ListPhotoColumns(r.Context(), r.URL.Query().Get("theme"), contentQuery(r))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		items := make([]photoColumnResponse, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, newPhotoColumnResponse(mediaSvc, item))
		}
		httpx.WriteJSONETag(w, r, photoColumnListResponse{
			Items: items, Page: page.Page, PageSize: page.PageSize,
			Total: page.Total, TotalPages: page.TotalPages,
		})
	}
}

func listColumnAlbumsHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		column, err := svc.GetPhotoColumn(r.Context(), slug)
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		page, err := svc.ListAlbums(r.Context(), slug, contentQuery(r))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		items := make([]albumResponse, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, newAlbumResponse(mediaSvc, item))
		}
		httpx.WriteJSONETag(w, r, columnAlbumsResponse{
			Column: columnRefResponse{Slug: column.Slug, Name: column.Name},
			Items:  items, Page: page.Page, PageSize: page.PageSize,
			Total: page.Total, TotalPages: page.TotalPages,
		})
	}
}

func getAlbumHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		album, err := svc.GetAlbum(r.Context(), chi.URLParam(r, "slug"))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		httpx.WriteJSONETag(w, r, newAlbumResponse(mediaSvc, album))
	}
}

func listAlbumPhotosHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := svc.ListPhotos(r.Context(), chi.URLParam(r, "slug"), "", contentQuery(r))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		writePhotoPage(w, r, mediaSvc, page)
	}
}

func listPhotosHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		page, err := svc.ListPhotos(r.Context(), query.Get("album"), query.Get("column"), contentQuery(r))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		writePhotoPage(w, r, mediaSvc, page)
	}
}

func writePhotoPage(w http.ResponseWriter, r *http.Request, mediaSvc *media.Service, page content.Page[content.Photo]) {
	items := make([]photoResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, newPhotoResponse(mediaSvc, item))
	}
	httpx.WriteJSONETag(w, r, photoListResponse{
		Items: items, Page: page.Page, PageSize: page.PageSize,
		Total: page.Total, TotalPages: page.TotalPages,
	})
}

func getPhotoHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		photo, err := svc.GetPhoto(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		httpx.WriteJSONETag(w, r, photoDetailResponse{
			photoResponse: newPhotoResponse(mediaSvc, photo),
			Exif:          photo.Media.Exif,
		})
	}
}

func searchHandler(svc *content.Service, mediaSvc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		types := splitList(r.URL.Query().Get("type"))
		page, err := svc.Search(r.Context(), contentQuery(r), types)
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		items := make([]searchHitResponse, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, searchHitResponse{
				Type:        item.Type,
				ID:          item.ID,
				Slug:        item.Slug,
				Title:       item.Title,
				Summary:     item.Summary,
				Cover:       newAssetResponse(mediaSvc, item.Cover),
				PublishedAt: item.PublishedAt,
				Column:      columnRefResponse{Slug: item.ColumnSlug, Name: item.ColumnName},
				Album:       columnRefResponse{Slug: item.AlbumSlug, Name: item.AlbumTitle},
			})
		}
		httpx.WriteJSONETag(w, r, searchListResponse{
			Items: items, Page: page.Page, PageSize: page.PageSize,
			Total: page.Total, TotalPages: page.TotalPages,
		})
	}
}

func listTagsHandler(svc *content.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		if scope == "" {
			scope = content.ScopeArticle
		}
		tags, err := svc.ListTags(r.Context(), scope)
		if err != nil {
			writeContentError(w, r, err)
			return
		}
		items := make([]tagResponse, 0, len(tags))
		for _, tag := range tags {
			items = append(items, tagResponse{Name: tag.Name})
		}
		httpx.WriteJSONETag(w, r, map[string]any{"scope": scope, "items": items})
	}
}

func contentQuery(r *http.Request) content.Query {
	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("pageSize"))
	return content.Query{
		Page:     page,
		PageSize: pageSize,
		Search:   query.Get("q"),
		Tags:     splitList(query.Get("tags")),
		DateFrom: query.Get("dateFrom"),
		DateTo:   query.Get("dateTo"),
		Sort:     query.Get("sort"),
	}
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func newAssetResponse(mediaSvc *media.Service, asset *content.MediaAsset) *assetResponse {
	if asset == nil || asset.ID == "" {
		return nil
	}
	variants := make([]assetVariantResponse, 0, len(asset.Variants))
	for _, name := range media.DisplayVariants {
		variant, ok := asset.Variants[name]
		if !ok {
			continue
		}
		variants = append(variants, assetVariantResponse{
			Name:   name,
			URL:    mediaSvc.VariantURL(variant.Key),
			Width:  variant.Width,
			Height: variant.Height,
			Size:   variant.Size,
		})
	}
	return &assetResponse{
		MediaID:  asset.ID,
		Key:      "media/" + asset.ID,
		Width:    asset.Width,
		Height:   asset.Height,
		Blurhash: asset.Blurhash,
		Variants: variants,
	}
}

func newArticleColumnResponse(mediaSvc *media.Service, column content.ArticleColumn) articleColumnResponse {
	return articleColumnResponse{
		ID:           column.ID,
		Slug:         column.Slug,
		Name:         column.Name,
		Description:  column.Description,
		Cover:        newAssetResponse(mediaSvc, column.Cover),
		Tags:         column.Tags,
		ArticleCount: column.ArticleCount,
		UpdatedAt:    column.UpdatedAt,
	}
}

func newArticleResponse(mediaSvc *media.Service, article content.Article) articleResponse {
	return articleResponse{
		ID:          article.ID,
		Slug:        article.Slug,
		Title:       article.Title,
		Summary:     article.Summary,
		Column:      columnRefResponse{Slug: article.Column.Slug, Name: article.Column.Name},
		Cover:       newAssetResponse(mediaSvc, article.Cover),
		Tags:        article.Tags,
		PublishedAt: article.PublishedAt,
	}
}

func newPhotoColumnResponse(mediaSvc *media.Service, column content.PhotoColumn) photoColumnResponse {
	return photoColumnResponse{
		ID:          column.ID,
		Slug:        column.Slug,
		Name:        column.Name,
		Description: column.Description,
		Theme:       column.Theme,
		Cover:       newAssetResponse(mediaSvc, column.Cover),
		Tags:        column.Tags,
		AlbumCount:  column.AlbumCount,
		UpdatedAt:   column.UpdatedAt,
	}
}

func newAlbumResponse(mediaSvc *media.Service, album content.Album) albumResponse {
	return albumResponse{
		ID:            album.ID,
		Slug:          album.Slug,
		Title:         album.Title,
		Description:   album.Description,
		Location:      album.Location,
		ShotAt:        album.ShotAt,
		Column:        columnRefResponse{Slug: album.Column.Slug, Name: album.Column.Name},
		Cover:         newAssetResponse(mediaSvc, album.Cover),
		Tags:          album.Tags,
		PhotoCount:    album.PhotoCount,
		LatestPhotoAt: album.LatestPhotoAt,
		PublishedAt:   album.PublishedAt,
	}
}

func newPhotoResponse(mediaSvc *media.Service, photo content.Photo) photoResponse {
	return photoResponse{
		ID:          photo.ID,
		Title:       photo.Title,
		Description: photo.Description,
		Location:    photo.Location,
		TakenAt:     photo.TakenAt,
		Album:       columnRefResponse{Slug: photo.Album.Slug, Name: photo.Album.Name},
		Column:      columnRefResponse{Slug: photo.Column.Slug, Name: photo.Column.Name},
		Tags:        photo.Tags,
		Media:       *newAssetResponse(mediaSvc, &photo.Media),
		PublishedAt: photo.PublishedAt,
	}
}

func writeContentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, content.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "内容不存在", nil)
	case errors.Is(err, content.ErrInvalidInput):
		httpx.WriteError(w, r, http.StatusBadRequest, "validation_error", content.Message(err), nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "服务器内部错误", nil)
	}
}
