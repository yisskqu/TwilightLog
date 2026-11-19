package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yisskqu/TwilightLog/services/api/internal/auth"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/httpx"
	"github.com/yisskqu/TwilightLog/services/api/internal/workspace"
)

func registerWorkspaceRoutes(r chi.Router, deps Deps) {
	r.Route("/workspace", func(r chi.Router) {
		r.Use(deps.Auth.RequireAuth, deps.Auth.RequireEditor)
		r.Get("/summary", workspaceSummaryHandler(deps.Workspace))
		r.Get("/content", workspaceListHandler(deps.Workspace))
		r.Post("/content", workspaceCreateHandler(deps.Workspace))
		r.Get("/content/{type}/{id}", workspaceDetailHandler(deps.Workspace))
		r.Put("/content/{type}/{id}", workspaceSaveHandler(deps.Workspace))
		r.Post("/content/{type}/{id}/submit", workspaceSubmitHandler(deps.Workspace))
		r.Post("/content/{type}/{id}/withdraw", workspaceWithdrawHandler(deps.Workspace))
		r.Post("/content/{type}/{id}/trash", workspaceTrashHandler(deps.Workspace))
		r.Post("/content/{type}/{id}/restore", workspaceRestoreHandler(deps.Workspace))
		r.Get("/recycle", workspaceRecycleHandler(deps.Workspace))
	})

	r.Route("/notifications", func(r chi.Router) {
		r.Use(deps.Auth.RequireAuth, deps.Auth.RequireEditor)
		r.Get("/", notificationsHandler(deps.Workspace))
		r.Post("/read-all", markAllNotificationsHandler(deps.Workspace))
		r.Post("/{id}/read", markNotificationHandler(deps.Workspace))
	})
}

// registerAdminWorkspaceRoutes mounts review, recommendation, tag and system
// routes inside the existing /admin group.
func registerAdminWorkspaceRoutes(r chi.Router, deps Deps) {
	r.Get("/reviews", reviewListHandler(deps.Workspace))
	r.Get("/reviews/{id}", reviewDetailHandler(deps.Workspace))
	r.Post("/reviews/{id}/approve", approveHandler(deps.Workspace))
	r.Post("/reviews/{id}/reject", rejectHandler(deps.Workspace))

	r.Get("/recommendations", recommendationHandler(deps.Workspace))
	r.Put("/recommendations", saveRecommendationsHandler(deps.Workspace))
	r.Get("/recommendations/candidates", recommendationCandidatesHandler(deps.Workspace))

	r.Get("/tags", adminTagsHandler(deps.Workspace))
	r.Patch("/tags/{id}", updateTagHandler(deps.Workspace))
	r.Post("/tags/{id}/merge", mergeTagHandler(deps.Workspace))
	r.Get("/system", systemInfoHandler(deps.Workspace))

	r.Delete("/content/{type}/{id}", purgeContentHandler(deps.Workspace))
}

func workspaceSummaryHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		result, err := svc.Summary(r.Context(), actor, r.URL.Query().Get("all") == "1")
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	}
}

func workspaceListHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		query := r.URL.Query()
		page, pageSize := pagination(r)
		result, err := svc.List(r.Context(), actor, workspace.ListQuery{
			Type:     query.Get("type"),
			Status:   query.Get("status"),
			Search:   query.Get("q"),
			Page:     page,
			PageSize: pageSize,
		}, query.Get("all") == "1")
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	}
}

func workspaceRecycleHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		query := r.URL.Query()
		page, pageSize := pagination(r)
		result, err := svc.List(r.Context(), actor, workspace.ListQuery{
			Type:     query.Get("type"),
			Status:   workspace.StatusTrashed,
			Search:   query.Get("q"),
			Page:     page,
			PageSize: pageSize,
		}, query.Get("all") == "1")
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	}
}

type workspaceCreateRequest struct {
	Type     string         `json:"type"`
	Slug     string         `json:"slug"`
	ParentID string         `json:"parentId"`
	Payload  map[string]any `json:"payload"`
}

func workspaceCreateHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload workspaceCreateRequest
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		detail, err := svc.Create(r.Context(), actor, payload.Type, workspace.SaveInput{
			Slug: payload.Slug, ParentID: payload.ParentID, Payload: payload.Payload,
		}, clientIP(r), r.UserAgent())
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, detail)
	}
}

func workspaceDetailHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		detail, err := svc.Detail(r.Context(), actor, chi.URLParam(r, "type"), chi.URLParam(r, "id"))
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, detail)
	}
}

func workspaceSaveHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Slug    string         `json:"slug"`
			Payload map[string]any `json:"payload"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		detail, err := svc.Save(r.Context(), actor, chi.URLParam(r, "type"), chi.URLParam(r, "id"),
			workspace.SaveInput{Slug: payload.Slug, Payload: payload.Payload}, clientIP(r), r.UserAgent())
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, detail)
	}
}

func workspaceSubmitHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		detail, err := svc.Submit(r.Context(), actor, chi.URLParam(r, "type"), chi.URLParam(r, "id"),
			clientIP(r), r.UserAgent())
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, detail)
	}
}

func workspaceWithdrawHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		detail, err := svc.Withdraw(r.Context(), actor, chi.URLParam(r, "type"), chi.URLParam(r, "id"),
			clientIP(r), r.UserAgent())
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, detail)
	}
}

func workspaceTrashHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		if err := svc.Trash(r.Context(), actor, chi.URLParam(r, "type"), chi.URLParam(r, "id"),
			clientIP(r), r.UserAgent()); err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func workspaceRestoreHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		if err := svc.Restore(r.Context(), actor, chi.URLParam(r, "type"), chi.URLParam(r, "id"),
			clientIP(r), r.UserAgent()); err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func reviewListHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := svc.Pending(r.Context(), r.URL.Query().Get("type"), r.URL.Query().Get("q"))
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func reviewDetailHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, err := svc.ReviewDetail(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, detail)
	}
}

func approveHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Comment    string   `json:"comment"`
			RemoveTags []string `json:"removeTags"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		versionID := chi.URLParam(r, "id")
		// Mobile clients may only approve a first publication; replacing an
		// existing public version is a desktop-only risk.
		if mobileRequest(r) {
			current, err := svc.ReviewDetail(r.Context(), versionID)
			if err != nil {
				writeWorkspaceError(w, r, err)
				return
			}
			if current.HasPublished {
				httpx.WriteError(w, r, http.StatusForbidden, "desktop_required", "替换公开版本需要在桌面端操作", nil)
				return
			}
		}
		actor, _ := auth.UserFromContext(r.Context())
		detail, removed, err := svc.Approve(r.Context(), actor, versionID, payload.Comment, payload.RemoveTags,
			clientIP(r), r.UserAgent())
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "review": detail, "removedTags": removed})
	}
}

func rejectHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Comment string `json:"comment"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		if _, err := svc.Reject(r.Context(), actor, chi.URLParam(r, "id"), payload.Comment,
			clientIP(r), r.UserAgent()); err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func recommendationHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.Recommendations(r.Context())
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, settings)
	}
}

func saveRecommendationsHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload workspace.RecommendationSettings
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		if err := svc.SaveRecommendations(r.Context(), actor, payload, clientIP(r), r.UserAgent()); err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func recommendationCandidatesHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := svc.Candidates(r.Context(), r.URL.Query().Get("type"))
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func adminTagsHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		if scope == "" {
			scope = "article"
		}
		items, err := svc.Tags(r.Context(), scope)
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"scope": scope, "items": items})
	}
}

func updateTagHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireDesktop(w, r) {
			return
		}
		var payload struct {
			Name   *string `json:"name"`
			Active *bool   `json:"active"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		if payload.Name != nil {
			if err := svc.RenameTag(r.Context(), actor, chi.URLParam(r, "id"), *payload.Name,
				clientIP(r), r.UserAgent()); err != nil {
				writeWorkspaceError(w, r, err)
				return
			}
		}
		if payload.Active != nil {
			if err := svc.SetTagActive(r.Context(), actor, chi.URLParam(r, "id"), *payload.Active,
				clientIP(r), r.UserAgent()); err != nil {
				writeWorkspaceError(w, r, err)
				return
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func mergeTagHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireDesktop(w, r) {
			return
		}
		var payload struct {
			TargetID string `json:"targetId"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		if err := svc.MergeTag(r.Context(), actor, chi.URLParam(r, "id"), payload.TargetID,
			clientIP(r), r.UserAgent()); err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func systemInfoHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, err := svc.SystemInfo(r.Context())
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, info)
	}
}

func purgeContentHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireDesktop(w, r) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		purged, err := svc.PermanentDelete(r.Context(), actor, chi.URLParam(r, "type"), chi.URLParam(r, "id"),
			clientIP(r), r.UserAgent())
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "mediaPurged": purged})
	}
}

func notificationsHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		page, pageSize := pagination(r)
		result, err := svc.Notifications(r.Context(), actor.ID, page, pageSize)
		if err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	}
}

func markNotificationHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		id := chi.URLParam(r, "id")
		if strings.TrimSpace(id) == "" {
			httpx.WriteError(w, r, http.StatusBadRequest, "validation_error", "通知 ID 不合法", nil)
			return
		}
		if err := svc.MarkRead(r.Context(), actor.ID, id); err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func markAllNotificationsHandler(svc *workspace.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		if err := svc.MarkAllRead(r.Context(), actor.ID); err != nil {
			writeWorkspaceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeWorkspaceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, workspace.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "内容不存在", nil)
	case errors.Is(err, workspace.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "无权操作该内容", nil)
	case errors.Is(err, workspace.ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, "conflict", workspace.Message(err), nil)
	case errors.Is(err, workspace.ErrInvalidInput):
		httpx.WriteError(w, r, http.StatusBadRequest, "validation_error", workspace.Message(err), nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "服务器内部错误", nil)
	}
}

func mobileRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Client-Device")), "mobile")
}

func requireDesktop(w http.ResponseWriter, r *http.Request) bool {
	if mobileRequest(r) {
		httpx.WriteError(w, r, http.StatusForbidden, "desktop_required", "该操作需要在桌面端完成", nil)
		return false
	}
	return true
}
