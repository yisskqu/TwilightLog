package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/yisskqu/TwilightLog/services/api/internal/audit"
	"github.com/yisskqu/TwilightLog/services/api/internal/auth"
	"github.com/yisskqu/TwilightLog/services/api/internal/media"
)

// Config carries the read-only deployment values shown in the admin.
type Config struct {
	AppEnv          string
	Processor       string
	MaxArticleBytes int64
	MaxPhotoBytes   int64
	BatchLimit      int
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	CookieSecure    bool
}

// Service applies ownership rules, review state changes and side effects.
type Service struct {
	repo  *Repository
	audit *audit.Service
	media *media.Service
	cfg   Config
}

func NewService(repo *Repository, auditService *audit.Service, mediaService *media.Service, cfg Config) *Service {
	return &Service{repo: repo, audit: auditService, media: mediaService, cfg: cfg}
}

// Summary is the editor dashboard block.
type Summary struct {
	Stats        []Stats `json:"stats"`
	PendingTotal int     `json:"pendingTotal"`
}

// List returns one type of content scoped to the actor unless all is true.
func (s *Service) List(ctx context.Context, actor auth.User, q ListQuery, all bool) (Page[Item], error) {
	if strings.TrimSpace(q.Type) == "" {
		return Page[Item]{}, fmt.Errorf("%w: 请指定内容类型", ErrInvalidInput)
	}
	info, err := TypeInfoFor(q.Type)
	if err != nil {
		return Page[Item]{}, err
	}
	q, err = q.Normalize()
	if err != nil {
		return Page[Item]{}, err
	}
	if !isAdmin(actor) || !all {
		q.OwnerID = actor.ID
	} else {
		q.OwnerID = ""
	}
	return s.repo.List(ctx, info, q)
}

// Summary counts every type for the workspace home.
func (s *Service) Summary(ctx context.Context, actor auth.User, all bool) (Summary, error) {
	owner := actor.ID
	if isAdmin(actor) && all {
		owner = ""
	}
	result := Summary{Stats: make([]Stats, 0, len(TypeCodes()))}
	for _, code := range TypeCodes() {
		info, _ := TypeInfoFor(code)
		stats, err := s.repo.Stats(ctx, info, owner)
		if err != nil {
			return Summary{}, err
		}
		pending, err := s.repo.PendingCount(ctx, info, owner)
		if err != nil {
			return Summary{}, err
		}
		result.PendingTotal += pending
		result.Stats = append(result.Stats, stats)
	}
	return result, nil
}

// Detail loads one entity when the actor may manage it.
func (s *Service) Detail(ctx context.Context, actor auth.User, code, id string) (Detail, error) {
	info, err := TypeInfoFor(code)
	if err != nil {
		return Detail{}, err
	}
	detail, err := s.repo.Get(ctx, info, id)
	if err != nil {
		return Detail{}, err
	}
	if !canManage(actor, detail.OwnerID) {
		return Detail{}, ErrForbidden
	}
	return detail, nil
}

// Create validates input, inserts the entity and returns its detail.
func (s *Service) Create(ctx context.Context, actor auth.User, code string, input SaveInput, ip, userAgent string) (Detail, error) {
	info, err := TypeInfoFor(code)
	if err != nil {
		return Detail{}, err
	}
	payload, tags, err := NormalizePayload(code, input.Payload)
	if err != nil {
		return Detail{}, err
	}
	parentID := strings.TrimSpace(input.ParentID)
	if err := s.validateParent(ctx, actor, info, parentID); err != nil {
		return Detail{}, err
	}
	if err := s.validateMedia(ctx, actor, info, payload); err != nil {
		return Detail{}, err
	}
	slug := s.buildSlug(info, input.Slug, TitleOf(code, payload))
	entityID, err := s.repo.Create(ctx, info, actor.ID, slug, parentID, payload, tags)
	if err != nil {
		return Detail{}, err
	}
	s.log(ctx, actor, "content.create", code, entityID, ip, userAgent, map[string]any{"title": TitleOf(code, payload)})
	return s.Detail(ctx, actor, code, entityID)
}

// Save writes the working draft.
func (s *Service) Save(ctx context.Context, actor auth.User, code, id string, input SaveInput, ip, userAgent string) (Detail, error) {
	info, err := TypeInfoFor(code)
	if err != nil {
		return Detail{}, err
	}
	owner, err := s.repo.Owner(ctx, info, id)
	if err != nil {
		return Detail{}, err
	}
	if !canManage(actor, owner) {
		return Detail{}, ErrForbidden
	}
	payload, tags, err := NormalizePayload(code, input.Payload)
	if err != nil {
		return Detail{}, err
	}
	if err := s.validateMedia(ctx, actor, info, payload); err != nil {
		return Detail{}, err
	}
	slug := ""
	if info.HasSlug {
		slug = s.buildSlug(info, input.Slug, TitleOf(code, payload))
	}
	if _, err := s.repo.Save(ctx, info, id, actor.ID, slug, payload, tags); err != nil {
		return Detail{}, err
	}
	s.log(ctx, actor, "content.save", code, id, ip, userAgent, map[string]any{"title": TitleOf(code, payload)})
	return s.Detail(ctx, actor, code, id)
}

// Submit moves the working draft into the review queue.
func (s *Service) Submit(ctx context.Context, actor auth.User, code, id, ip, userAgent string) (Detail, error) {
	info, err := TypeInfoFor(code)
	if err != nil {
		return Detail{}, err
	}
	owner, err := s.repo.Owner(ctx, info, id)
	if err != nil {
		return Detail{}, err
	}
	if !canManage(actor, owner) {
		return Detail{}, ErrForbidden
	}
	version, err := s.repo.Submit(ctx, info, id, actor.ID)
	if err != nil {
		return Detail{}, err
	}
	title := TitleOf(code, version.Payload)
	s.log(ctx, actor, "content.submit", code, id, ip, userAgent, map[string]any{"versionId": version.ID})
	_ = s.repo.NotifyRole(ctx, []string{"admin", "super_admin"}, "content.submitted", "info",
		"新的待审内容："+title, fmt.Sprintf("%s 提交了%s，请前往审批中心。", actor.Username, typeLabel(code)),
		"/admin/reviews?version="+version.ID)
	return s.Detail(ctx, actor, code, id)
}

// Withdraw cancels a pending submission.
func (s *Service) Withdraw(ctx context.Context, actor auth.User, code, id, ip, userAgent string) (Detail, error) {
	info, err := TypeInfoFor(code)
	if err != nil {
		return Detail{}, err
	}
	owner, err := s.repo.Owner(ctx, info, id)
	if err != nil {
		return Detail{}, err
	}
	if !canManage(actor, owner) {
		return Detail{}, ErrForbidden
	}
	if _, err := s.repo.Withdraw(ctx, info, id); err != nil {
		return Detail{}, err
	}
	s.log(ctx, actor, "content.withdraw", code, id, ip, userAgent, nil)
	return s.Detail(ctx, actor, code, id)
}

// Pending lists the review queue, optionally scoped to one type.
func (s *Service) Pending(ctx context.Context, code, search string) ([]ReviewItem, error) {
	if code != "" {
		if _, err := TypeInfoFor(code); err != nil {
			return nil, err
		}
	}
	return s.repo.Pending(ctx, code, search)
}

// ReviewDetail loads one submission.
func (s *Service) ReviewDetail(ctx context.Context, versionID string) (ReviewDetail, error) {
	return s.repo.ReviewDetail(ctx, versionID)
}

// Approve publishes a pending version.
func (s *Service) Approve(ctx context.Context, actor auth.User, versionID, comment string, removeTags []string, ip, userAgent string) (ReviewDetail, []string, error) {
	detail, removed, err := s.repo.Approve(ctx, versionID, actor.ID, comment, removeTags)
	if err != nil {
		return ReviewDetail{}, nil, err
	}
	s.log(ctx, actor, "content.approve", detail.Type, detail.EntityID, ip, userAgent,
		map[string]any{"versionId": versionID, "removedTags": removed})
	if detail.AuthorID != "" {
		_ = s.repo.NotifyUser(ctx, detail.AuthorID, "content.approved", "info",
			"内容已通过："+detail.Title, "你的提交已替换公开版本。", "/notifications")
	}
	return detail, removed, nil
}

// Reject sends a submission back with a required reason.
func (s *Service) Reject(ctx context.Context, actor auth.User, versionID, comment, ip, userAgent string) (ReviewDetail, error) {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return ReviewDetail{}, fmt.Errorf("%w: 驳回理由不能为空", ErrInvalidInput)
	}
	detail, err := s.repo.Reject(ctx, versionID, actor.ID, comment)
	if err != nil {
		return ReviewDetail{}, err
	}
	s.log(ctx, actor, "content.reject", detail.Type, detail.EntityID, ip, userAgent,
		map[string]any{"versionId": versionID, "comment": comment})
	if detail.AuthorID != "" {
		_ = s.repo.NotifyUser(ctx, detail.AuthorID, "content.rejected", "warning",
			"内容被驳回："+detail.Title, "理由："+comment, "/notifications")
	}
	return detail, nil
}

// Trash moves one entity to the recycle bin.
func (s *Service) Trash(ctx context.Context, actor auth.User, code, id, ip, userAgent string) error {
	info, err := TypeInfoFor(code)
	if err != nil {
		return err
	}
	owner, err := s.repo.Owner(ctx, info, id)
	if err != nil {
		return err
	}
	if !canManage(actor, owner) {
		return ErrForbidden
	}
	if err := s.repo.Trash(ctx, info, id); err != nil {
		return err
	}
	s.log(ctx, actor, "content.trash", code, id, ip, userAgent, nil)
	return nil
}

// Restore returns one entity from the recycle bin.
func (s *Service) Restore(ctx context.Context, actor auth.User, code, id, ip, userAgent string) error {
	info, err := TypeInfoFor(code)
	if err != nil {
		return err
	}
	owner, err := s.repo.Owner(ctx, info, id)
	if err != nil {
		return err
	}
	if !canManage(actor, owner) {
		return ErrForbidden
	}
	if err := s.repo.Restore(ctx, info, id); err != nil {
		return err
	}
	s.log(ctx, actor, "content.restore", code, id, ip, userAgent, nil)
	return nil
}

// PermanentDelete removes a trashed entity and purges unreferenced media.
func (s *Service) PermanentDelete(ctx context.Context, actor auth.User, code, id, ip, userAgent string) (int, error) {
	if !isAdmin(actor) {
		return 0, ErrForbidden
	}
	info, err := TypeInfoFor(code)
	if err != nil {
		return 0, err
	}
	mediaIDs, err := s.repo.PermanentDelete(ctx, info, id)
	if err != nil {
		return 0, err
	}
	purged := 0
	for _, mediaID := range mediaIDs {
		referenced, err := s.repo.MediaStillReferenced(ctx, mediaID, code, id)
		if err != nil {
			return purged, err
		}
		if referenced || s.media == nil {
			continue
		}
		if err := s.media.Purge(ctx, mediaID); err != nil {
			return purged, err
		}
		purged++
	}
	s.log(ctx, actor, "content.purge", code, id, ip, userAgent, map[string]any{"mediaPurged": purged})
	return purged, nil
}

// Notifications returns the current user's inbox.
func (s *Service) Notifications(ctx context.Context, userID string, page, pageSize int) (NotificationPage, error) {
	return s.repo.ListNotifications(ctx, userID, page, pageSize)
}

// Unread returns the bell badge count.
func (s *Service) Unread(ctx context.Context, userID string) (int, error) {
	return s.repo.UnreadNotifications(ctx, userID)
}

// MarkRead marks one notification read.
func (s *Service) MarkRead(ctx context.Context, userID string, id string) error {
	return s.repo.MarkNotificationRead(ctx, userID, id)
}

// MarkAllRead clears the inbox badge.
func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	return s.repo.MarkAllNotificationsRead(ctx, userID)
}

// NotifyAccount records an account-level notification for one user.
func (s *Service) NotifyAccount(ctx context.Context, userID, kind, title, body string) {
	if userID == "" {
		return
	}
	_ = s.repo.NotifyUser(ctx, userID, kind, "info", title, body, "/notifications")
}

// Tags lists the tag library for one scope.
func (s *Service) Tags(ctx context.Context, scope string) ([]TagUsage, error) {
	if scope != "article" && scope != "photo" {
		return nil, fmt.Errorf("%w: 作用域必须是 article 或 photo", ErrInvalidInput)
	}
	return s.repo.ListTags(ctx, scope)
}

// RenameTag updates one tag name.
func (s *Service) RenameTag(ctx context.Context, actor auth.User, id, name, ip, userAgent string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > MaxTagLength || strings.Contains(name, ",") {
		return fmt.Errorf("%w: 标签名不合法", ErrInvalidInput)
	}
	if err := s.repo.RenameTag(ctx, id, name); err != nil {
		return err
	}
	s.log(ctx, actor, "tag.rename", "tag", id, ip, userAgent, map[string]any{"name": name})
	return nil
}

// MergeTag moves links from one tag to another in the same scope.
func (s *Service) MergeTag(ctx context.Context, actor auth.User, sourceID, targetID, ip, userAgent string) error {
	if err := s.repo.MergeTag(ctx, sourceID, targetID); err != nil {
		return err
	}
	s.log(ctx, actor, "tag.merge", "tag", sourceID, ip, userAgent, map[string]any{"targetId": targetID})
	return nil
}

// SetTagActive enables or disables a tag.
func (s *Service) SetTagActive(ctx context.Context, actor auth.User, id string, active bool, ip, userAgent string) error {
	if err := s.repo.SetTagActive(ctx, id, active); err != nil {
		return err
	}
	action := "tag.disable"
	if active {
		action = "tag.enable"
	}
	s.log(ctx, actor, action, "tag", id, ip, userAgent, nil)
	return nil
}

// Recommendations returns the configured home page selection.
func (s *Service) Recommendations(ctx context.Context) (RecommendationSettings, error) {
	return s.repo.RecommendationSettings(ctx)
}

// SaveRecommendations validates and replaces the home page selection.
func (s *Service) SaveRecommendations(ctx context.Context, actor auth.User, settings RecommendationSettings, ip, userAgent string) error {
	if settings.N1 < 0 || settings.N1 > 12 || settings.N2 < 0 || settings.N2 > 12 {
		return fmt.Errorf("%w: N₁、N₂ 必须在 0 到 12 之间", ErrInvalidInput)
	}
	articles, albums := 0, 0
	seen := map[string]struct{}{}
	for _, item := range settings.Items {
		switch item.EntityType {
		case TypeArticle:
			articles++
		case TypeAlbum:
			albums++
		default:
			return fmt.Errorf("%w: 推荐只支持文章与相册", ErrInvalidInput)
		}
		key := item.EntityType + ":" + item.EntityID
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: 推荐项不能重复", ErrInvalidInput)
		}
		seen[key] = struct{}{}
		title, visible, err := s.repo.PublicTarget(ctx, item.EntityType, item.EntityID)
		if err != nil {
			return err
		}
		if !visible {
			return fmt.Errorf("%w: 只能选择当前公开可见的内容（%s）", ErrInvalidInput, title)
		}
	}
	if articles > settings.N1 {
		return fmt.Errorf("%w: 文章推荐最多 %d 项，当前 %d 项", ErrInvalidInput, settings.N1, articles)
	}
	if albums > settings.N2 {
		return fmt.Errorf("%w: 相册推荐最多 %d 项，当前 %d 项", ErrInvalidInput, settings.N2, albums)
	}
	if err := s.repo.SaveRecommendationSettings(ctx, settings); err != nil {
		return err
	}
	s.log(ctx, actor, "recommendation.save", "recommendation", "", ip, userAgent,
		map[string]any{"n1": settings.N1, "n2": settings.N2, "autoFill": settings.AutoFill, "count": len(settings.Items)})
	return nil
}

// Candidates lists public articles or albums for the recommendation picker.
func (s *Service) Candidates(ctx context.Context, entityType string) ([]Candidate, error) {
	if entityType != TypeArticle && entityType != TypeAlbum {
		return nil, fmt.Errorf("%w: 推荐只支持文章与相册", ErrInvalidInput)
	}
	return s.repo.Candidates(ctx, entityType)
}

// SystemInfo returns the read-only deployment snapshot.
func (s *Service) SystemInfo(ctx context.Context) (SystemInfo, error) {
	info, err := s.repo.SystemInfo(ctx)
	if err != nil {
		return SystemInfo{}, err
	}
	info.AppEnv = s.cfg.AppEnv
	info.MaxArticleMB = s.cfg.MaxArticleBytes >> 20
	info.MaxPhotoMB = s.cfg.MaxPhotoBytes >> 20
	info.BatchLimit = s.cfg.BatchLimit
	info.Processor = s.cfg.Processor
	info.AccessTokenTTL = s.cfg.AccessTokenTTL.String()
	info.RefreshTokenTTL = s.cfg.RefreshTokenTTL.String()
	info.CookieSecure = s.cfg.CookieSecure
	info.Watermark = "右下角，宽度 5–7%，最小 24px、最大 120px，透明度 55–70%"
	return info, nil
}

func (s *Service) validateParent(ctx context.Context, actor auth.User, info TypeInfo, parentID string) error {
	if info.ParentType == "" {
		return nil
	}
	if parentID == "" {
		return fmt.Errorf("%w: 必须指定上级内容", ErrInvalidInput)
	}
	parent, err := s.repo.Parent(ctx, info, parentID)
	if err != nil {
		return err
	}
	if !canManage(actor, parent.OwnerID) {
		return ErrForbidden
	}
	return nil
}

func (s *Service) validateMedia(ctx context.Context, actor auth.User, info TypeInfo, payload map[string]any) error {
	for _, mediaID := range MediaIDs(payload) {
		owner, purpose, status, err := s.repo.MediaInfo(ctx, mediaID)
		if err != nil {
			return err
		}
		if status != "ready" {
			return fmt.Errorf("%w: 图片尚未处理完成", ErrInvalidInput)
		}
		if !isAdmin(actor) && owner != actor.ID {
			return fmt.Errorf("%w: 不能使用他人上传的图片", ErrForbidden)
		}
		if info.Code == TypePhoto && purpose != "photo" {
			return fmt.Errorf("%w: 照片必须使用摄影用途的图片", ErrInvalidInput)
		}
	}
	return nil
}

func (s *Service) buildSlug(info TypeInfo, requested, title string) string {
	if !info.HasSlug {
		return ""
	}
	slug := Slugify(requested)
	if slug == "" {
		slug = Slugify(title)
	}
	if slug == "" {
		slug = strings.TrimSuffix(info.Code, "_column")
	}
	if len(slug) < 3 {
		slug = slug + "-" + randomSuffix()
	}
	return slug
}

func (s *Service) log(ctx context.Context, actor auth.User, action, targetType, targetID, ip, userAgent string, metadata map[string]any) {
	if s.audit == nil {
		return
	}
	actorID := actor.ID
	s.audit.Log(ctx, &actorID, action, targetType, targetID, "success", ip, userAgent, metadata)
}

func isAdmin(user auth.User) bool {
	return user.Role == "super_admin" || user.Role == "admin"
}

func canManage(user auth.User, ownerID string) bool {
	return isAdmin(user) || (ownerID != "" && user.ID == ownerID)
}

func typeLabel(code string) string {
	switch code {
	case TypeArticleColumn:
		return "文章专栏"
	case TypeArticle:
		return "文章"
	case TypePhotoColumn:
		return "摄影专栏"
	case TypeAlbum:
		return "相册"
	case TypePhoto:
		return "照片"
	default:
		return "内容"
	}
}

func randomSuffix() string {
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	}
	return hex.EncodeToString(buf[:])
}
