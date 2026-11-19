import { useCallback, useEffect, useRef, useState } from "react";
import type { PhotoDetail, PhotoSummary } from "../lib/api";
import { imageSource } from "../lib/format";

export interface PhotoViewerProps {
  photos: PhotoSummary[];
  index: number | null;
  onIndexChange: (index: number) => void;
  onClose: () => void;
}

const MAX_SCALE = 4;

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export default function PhotoViewer({
  photos,
  index,
  onIndexChange,
  onClose,
}: PhotoViewerProps) {
  const open = index !== null && photos.length > 0;
  const photo = open ? photos[index as number] : null;
  const [detail, setDetail] = useState<PhotoDetail | null>(null);
  const [scale, setScale] = useState(1);
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [dragging, setDragging] = useState(false);

  const rootRef = useRef<HTMLDivElement | null>(null);
  const closeRef = useRef<HTMLButtonElement | null>(null);
  const stageRef = useRef<HTMLDivElement | null>(null);
  const restoreFocus = useRef<Element | null>(null);
  const dragState = useRef<{ x: number; y: number; originX: number; originY: number } | null>(
    null,
  );

  const resetZoom = useCallback(() => {
    setScale(1);
    setOffset({ x: 0, y: 0 });
  }, []);

  const goPrev = useCallback(() => {
    if (index === null) return;
    if (index > 0) onIndexChange(index - 1);
  }, [index, onIndexChange]);

  const goNext = useCallback(() => {
    if (index === null) return;
    if (index < photos.length - 1) onIndexChange(index + 1);
  }, [index, photos.length, onIndexChange]);

  useEffect(() => {
    resetZoom();
    setDetail(null);
    if (!photo) return;
    let cancelled = false;
    fetch(`/api/v1/photos/${encodeURIComponent(photo.id)}`, {
      headers: { Accept: "application/json" },
    })
      .then((response) => (response.ok ? response.json() : null))
      .then((data: PhotoDetail | null) => {
        if (!cancelled) setDetail(data);
      })
      .catch(() => {
        if (!cancelled) setDetail(null);
      });
    return () => {
      cancelled = true;
    };
  }, [photo, resetZoom]);

  useEffect(() => {
    if (!open) return;
    restoreFocus.current = document.activeElement;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    return () => {
      document.body.style.overflow = previousOverflow;
      if (restoreFocus.current instanceof HTMLElement) {
        restoreFocus.current.focus();
      }
    };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        goPrev();
        return;
      }
      if (event.key === "ArrowRight") {
        event.preventDefault();
        goNext();
        return;
      }
      if (event.key === "+" || event.key === "=") {
        event.preventDefault();
        setScale((value) => clamp(value * 1.2, 1, MAX_SCALE));
        return;
      }
      if (event.key === "-") {
        event.preventDefault();
        setScale((value) => {
          const next = clamp(value / 1.2, 1, MAX_SCALE);
          if (next === 1) setOffset({ x: 0, y: 0 });
          return next;
        });
        return;
      }
      if (event.key === "0") {
        event.preventDefault();
        resetZoom();
        return;
      }
      if (event.key === "Tab" && rootRef.current) {
        const focusable = rootRef.current.querySelectorAll<HTMLElement>(
          'button:not(:disabled), [href], [tabindex]:not([tabindex="-1"])',
        );
        if (focusable.length === 0) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, onClose, goPrev, goNext, resetZoom]);

  useEffect(() => {
    if (!open) return;
    const stage = stageRef.current;
    if (!stage) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      setScale((value) => {
        const next = clamp(value * (event.deltaY < 0 ? 1.12 : 0.89), 1, MAX_SCALE);
        if (next === 1) {
          setOffset({ x: 0, y: 0 });
        }
        return next;
      });
    };
    stage.addEventListener("wheel", onWheel, { passive: false });
    return () => stage.removeEventListener("wheel", onWheel);
  }, [open]);

  if (!open || !photo) {
    return null;
  }

  const image = imageSource(photo.media, "w2048");
  const exif = detail?.exif ?? {};
  const exifEntries = [
    { label: "相机", value: [exif.cameraMake, exif.cameraModel].filter(Boolean).join(" ") },
    { label: "镜头", value: exif.lensModel ?? "" },
    { label: "焦距", value: exif.focalLength ?? "" },
    { label: "光圈", value: exif.aperture ?? "" },
    { label: "快门", value: exif.shutterSpeed ?? "" },
    { label: "ISO", value: exif.iso ? String(exif.iso) : "" },
    { label: "曝光补偿", value: exif.exposureBias ?? "" },
    { label: "白平衡", value: exif.whiteBalance ?? "" },
  ].filter((entry) => entry.value);

  const dateLabel = photo.takenAt || "";
  const albumLabel = [photo.column?.name, photo.album?.name].filter(Boolean).join(" · ");

  return (
    <div
      ref={rootRef}
      className="viewer"
      role="dialog"
      aria-modal="true"
      aria-label={`${photo.title} 照片查看`}
    >
      <div className="viewer__top">
        <div>
          <h2 className="viewer__title">{photo.title}</h2>
          <p className="viewer__count">
            {(index as number) + 1} / {photos.length}
            {albumLabel ? ` · ${albumLabel}` : ""}
          </p>
        </div>
        <button
          ref={closeRef}
          type="button"
          className="viewer__close"
          aria-label="关闭照片查看"
          onClick={onClose}
        >
          ×
        </button>
      </div>

      <div
        ref={stageRef}
        className={`viewer__stage${scale > 1 ? " is-zoomed" : ""}${dragging ? " is-dragging" : ""}`}
        onDoubleClick={resetZoom}
        onPointerDown={(event) => {
          if (scale <= 1) return;
          (event.target as HTMLElement).setPointerCapture?.(event.pointerId);
          dragState.current = {
            x: event.clientX,
            y: event.clientY,
            originX: offset.x,
            originY: offset.y,
          };
          setDragging(true);
        }}
        onPointerMove={(event) => {
          const state = dragState.current;
          if (!state) return;
          setOffset({
            x: state.originX + (event.clientX - state.x),
            y: state.originY + (event.clientY - state.y),
          });
        }}
        onPointerUp={() => {
          dragState.current = null;
          setDragging(false);
        }}
        onPointerCancel={() => {
          dragState.current = null;
          setDragging(false);
        }}
      >
        <button
          type="button"
          className="viewer__nav prev"
          aria-label="上一张"
          disabled={(index as number) <= 0}
          onClick={goPrev}
        >
          ‹
        </button>
        {image ? (
          <img
            className="viewer__image"
            src={image.src}
            srcSet={image.srcset}
            sizes="100vw"
            width={image.width}
            height={image.height}
            alt={photo.title}
            draggable={false}
            style={{
              transform: `translate(${offset.x}px, ${offset.y}px) scale(${scale})`,
            }}
          />
        ) : null}
        <button
          type="button"
          className="viewer__nav next"
          aria-label="下一张"
          disabled={(index as number) >= photos.length - 1}
          onClick={goNext}
        >
          ›
        </button>
      </div>

      <div className="viewer__bottom">
        <div className="viewer__info">
          <div>
            {photo.description ? <p className="viewer__description">{photo.description}</p> : null}
            <p className="viewer__hint" style={{ marginTop: 8 }}>
              {[photo.location, dateLabel, ...(photo.tags ?? [])].filter(Boolean).join(" · ")}
            </p>
            <p className="viewer__hint" style={{ marginTop: 6 }}>
              滚轮缩放 · 拖动查看 · 双击复位 · ←/→ 切换 · Esc 关闭
            </p>
          </div>
          <div>
            {exifEntries.length ? (
              <div className="exif-grid">
                {exifEntries.map((entry) => (
                  <p key={entry.label}>
                    <b>{entry.label}</b>
                    {entry.value}
                  </p>
                ))}
              </div>
            ) : (
              <p className="viewer__hint">该照片暂无公开的拍摄信息。</p>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
