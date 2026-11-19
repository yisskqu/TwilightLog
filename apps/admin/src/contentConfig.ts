import type { ContentType } from "./api";

export type FieldKind = "text" | "textarea" | "date" | "tags" | "media";

export type FieldDef = {
  key: string;
  label: string;
  kind: FieldKind;
  placeholder?: string;
  required?: boolean;
  rows?: number;
};

export type TypeConfig = {
  code: ContentType;
  label: string;
  plural: string;
  scope: "article" | "photo";
  parent?: ContentType;
  titleField: "name" | "title";
  hasSlug: boolean;
  fields: FieldDef[];
};

export const CONTENT_TYPES: Record<ContentType, TypeConfig> = {
  article_column: {
    code: "article_column",
    label: "文章专栏",
    plural: "文章专栏",
    scope: "article",
    titleField: "name",
    hasSlug: true,
    fields: [
      { key: "name", label: "名称", kind: "text", required: true },
      { key: "description", label: "简介", kind: "textarea", rows: 3 },
      { key: "tags", label: "主题标签", kind: "tags", placeholder: "用逗号分隔" },
      { key: "coverMediaId", label: "封面图", kind: "media" },
    ],
  },
  article: {
    code: "article",
    label: "文章",
    plural: "文章",
    scope: "article",
    parent: "article_column",
    titleField: "title",
    hasSlug: true,
    fields: [
      { key: "title", label: "标题", kind: "text", required: true },
      { key: "summary", label: "摘要", kind: "textarea", rows: 2 },
      { key: "bodyMarkdown", label: "正文（Markdown）", kind: "textarea", rows: 18 },
      { key: "tags", label: "标签", kind: "tags", placeholder: "用逗号分隔" },
      { key: "coverMediaId", label: "封面图", kind: "media" },
    ],
  },
  photo_column: {
    code: "photo_column",
    label: "摄影专栏",
    plural: "摄影专栏",
    scope: "photo",
    titleField: "name",
    hasSlug: true,
    fields: [
      { key: "name", label: "名称", kind: "text", required: true },
      { key: "description", label: "简介", kind: "textarea", rows: 3 },
      { key: "theme", label: "题材", kind: "text", placeholder: "例如 城市、自然" },
      { key: "tags", label: "标签", kind: "tags", placeholder: "用逗号分隔" },
      { key: "coverMediaId", label: "封面图", kind: "media" },
    ],
  },
  album: {
    code: "album",
    label: "相册",
    plural: "相册",
    scope: "photo",
    parent: "photo_column",
    titleField: "title",
    hasSlug: true,
    fields: [
      { key: "title", label: "名称", kind: "text", required: true },
      { key: "description", label: "简介", kind: "textarea", rows: 3 },
      { key: "location", label: "位置", kind: "text" },
      { key: "shotAt", label: "拍摄日期", kind: "date" },
      { key: "tags", label: "标签", kind: "tags", placeholder: "用逗号分隔" },
      { key: "coverMediaId", label: "封面图", kind: "media" },
    ],
  },
  photo: {
    code: "photo",
    label: "照片",
    plural: "照片",
    scope: "photo",
    parent: "album",
    titleField: "title",
    hasSlug: false,
    fields: [
      { key: "mediaId", label: "图片", kind: "media", required: true },
      { key: "title", label: "标题", kind: "text" },
      { key: "description", label: "描述", kind: "textarea", rows: 3 },
      { key: "location", label: "位置", kind: "text" },
      { key: "takenAt", label: "拍摄日期", kind: "date" },
      { key: "tags", label: "标签", kind: "tags", placeholder: "用逗号分隔" },
    ],
  },
};

export const TYPE_ORDER: ContentType[] = [
  "article_column",
  "article",
  "photo_column",
  "album",
  "photo",
];

export const STATUS_LABELS: Record<string, string> = {
  draft: "草稿",
  pending: "待审核",
  rejected: "已驳回",
  published: "已发布",
  offline: "已下线",
  trashed: "回收站",
  approved: "已通过",
};

export function isContentType(value: string | undefined): value is ContentType {
  return value !== undefined && value in CONTENT_TYPES;
}

export function payloadTags(payload: Record<string, unknown>): string[] {
  const value = payload.tags;
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

export function payloadString(payload: Record<string, unknown>, key: string): string {
  const value = payload[key];
  return typeof value === "string" ? value : "";
}
