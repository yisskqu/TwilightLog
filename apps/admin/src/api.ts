export type Role = "super_admin" | "admin" | "editor" | "user";

export type User = {
  id: string;
  username: string;
  email: string;
  role: Role;
  status: "active" | "disabled";
  createdAt: string;
  permissions: string[];
};

export type Account = {
  id: string;
  username: string;
  email: string;
  role: Role;
  status: "active" | "disabled";
  canManage: boolean;
  createdAt: string;
};

export type Page<T> = {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
};

export type AuditEntry = {
  id: number;
  actorId: string | null;
  actorName: string;
  action: string;
  targetType: string;
  targetId: string;
  result: "success" | "failure";
  ip: string;
  userAgent: string;
  metadata: Record<string, unknown>;
  createdAt: string;
};

export type ContentType = "article_column" | "article" | "photo_column" | "album" | "photo";

export type WorkspaceStatus = "draft" | "pending" | "rejected" | "published" | "offline" | "trashed";

export type WorkspaceItem = {
  id: string;
  type: ContentType;
  slug: string;
  title: string;
  status: WorkspaceStatus;
  workingStatus: string;
  versionId: string;
  versionNo: number;
  hasPublished: boolean;
  parentId: string;
  parentTitle: string;
  ownerId: string;
  ownerName: string;
  updatedAt: string;
  trashedAt: string | null;
  mediaCount: number;
};

export type WorkspaceStats = {
  type: ContentType;
  total: number;
  draft: number;
  pending: number;
  rejected: number;
  published: number;
  offline: number;
  trashed: number;
};

export type WorkspaceSummary = {
  stats: WorkspaceStats[];
  pendingTotal: number;
};

export type WorkspacePage = Page<WorkspaceItem> & { totalPages: number };

export type ContentVersion = {
  id: string;
  status: "draft" | "pending" | "rejected" | "approved";
  versionNo: number;
  payload: Record<string, unknown>;
  comment: string;
  updatedAt: string;
  submittedAt: string | null;
  publishedAt: string | null;
};

export type WorkspaceDetail = WorkspaceItem & {
  working: ContentVersion | null;
  published: ContentVersion | null;
};

export type ReviewItem = {
  versionId: string;
  type: ContentType;
  entityId: string;
  title: string;
  versionNo: number;
  status: string;
  authorId: string;
  authorName: string;
  submittedAt: string | null;
  updatedAt: string;
  hasPublished: boolean;
  payload: Record<string, unknown>;
};

export type ReviewDetail = ReviewItem & {
  published: Record<string, unknown>;
  tags: string[];
  publishedAt: string | null;
};

export type NotificationItem = {
  id: string;
  type: string;
  severity: "info" | "warning" | "error" | "success";
  title: string;
  body: string;
  link: string;
  readAt: string | null;
  createdAt: string;
};

export type NotificationPage = Page<NotificationItem> & { unread: number };

export type RecommendationItem = {
  position: number;
  entityType: "article" | "album";
  entityId: string;
  title: string;
};

export type RecommendationSettings = {
  n1: number;
  n2: number;
  autoFill: boolean;
  items: RecommendationItem[];
};

export type Candidate = {
  entityType: "article" | "album";
  entityId: string;
  title: string;
  updatedAt: string;
};

export type TagUsage = {
  id: string;
  scope: "article" | "photo";
  name: string;
  active: boolean;
  usage: number;
};

export type SystemInfo = {
  version: string;
  buildStatus: string;
  storageUsage: number;
  jobStatus: Record<string, unknown>;
  updatedAt: string;
  appEnv: string;
  maxArticleMB: number;
  maxPhotoMB: number;
  batchLimit: number;
  processor: string;
  accessTokenTTL: string;
  refreshTokenTTL: string;
  cookieSecure: boolean;
  watermark: string;
};

export function isMobileViewport(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(max-width: 860px)").matches;
}


type SessionResponse = {
  accessToken: string;
  expiresIn: number;
  user: User;
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string = "unknown",
  ) {
    super(message);
    this.name = "ApiError";
  }
}

let accessToken: string | null = null;

export function setAccessToken(token: string | null) {
  accessToken = token;
}

async function toApiError(response: Response): Promise<ApiError> {
  try {
    const body = (await response.json()) as { message?: string; code?: string };
    return new ApiError(body.message ?? "请求失败", response.status, body.code ?? "unknown");
  } catch {
    return new ApiError(`请求失败（HTTP ${response.status}）`, response.status);
  }
}

async function rawRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }
  headers.set("X-Client-Device", isMobileViewport() ? "mobile" : "desktop");

  const response = await fetch(path, {
    ...init,
    headers,
    credentials: "include",
  });

  if (!response.ok) {
    throw await toApiError(response);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

/** Restores the session from the HttpOnly refresh cookie. */
export async function refreshSession(): Promise<User | null> {
  const response = await fetch("/api/v1/auth/refresh", { method: "POST", credentials: "include" });
  if (!response.ok) {
    accessToken = null;
    return null;
  }
  const data = (await response.json()) as SessionResponse;
  accessToken = data.accessToken;
  return data.user;
}

/** Sends an authenticated request and retries once after refreshing the session. */
export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  try {
    return await rawRequest<T>(path, init);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      const user = await refreshSession();
      if (user) {
        return await rawRequest<T>(path, init);
      }
    }
    throw error;
  }
}

export async function loginRequest(username: string, password: string, remember: boolean): Promise<User> {
  const data = await rawRequest<SessionResponse>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password, remember }),
  });
  accessToken = data.accessToken;
  return data.user;
}

export async function logoutRequest(): Promise<void> {
  try {
    await rawRequest("/api/v1/auth/logout", { method: "POST" });
  } finally {
    accessToken = null;
  }
}

export async function fetchMe(): Promise<User> {
  return apiRequest<User>("/api/v1/auth/me");
}
