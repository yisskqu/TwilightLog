from pathlib import Path
import random

import piexif
import pillow_heif
from PIL import Image, ImageDraw


ROOT = Path(__file__).resolve().parent / "images"
ROOT.mkdir(parents=True, exist_ok=True)


def make_image(size: tuple[int, int]) -> Image.Image:
    random.seed(42)
    image = Image.new("RGB", size, (238, 232, 220))
    draw = ImageDraw.Draw(image)
    for _ in range(120):
        x1 = random.randint(0, size[0])
        y1 = random.randint(0, size[1])
        x2 = x1 + random.randint(40, 320)
        y2 = y1 + random.randint(40, 240)
        color = (
            random.randint(30, 220),
            random.randint(30, 220),
            random.randint(30, 220),
        )
        draw.rectangle((x1, y1, x2, y2), fill=color)
    draw.text((40, 40), "TwilightLog test image", fill=(20, 20, 20))
    return image


def pad_to_size(source: Path, target: Path, size: int) -> None:
    data = source.read_bytes()
    if len(data) >= size:
        raise RuntimeError(f"{source.name} is already larger than {size} bytes")
    with target.open("wb") as fh:
        fh.write(data)
        fh.write(b"\0" * (size - len(data)))


def main() -> None:
    image = make_image((1600, 1067))

    normal_jpg = ROOT / "normal.jpg"
    image.save(normal_jpg, "JPEG", quality=90, optimize=True)

    normal_png = ROOT / "normal.png"
    image.save(normal_png, "PNG", optimize=True)

    heic = ROOT / "sample.heic"
    pillow_heif.from_pillow(image).save(heic, quality=80)

    gps_exif = {
        "0th": {
            piexif.ImageIFD.Make: b"TwilightLogTest",
            piexif.ImageIFD.Model: b"TestCamera",
        },
        "Exif": {},
        "GPS": {
            piexif.GPSIFD.GPSVersionID: (2, 3, 0, 0),
            piexif.GPSIFD.GPSLatitudeRef: b"N",
            piexif.GPSIFD.GPSLatitude: ((31, 1), (12, 1), (3456, 100)),
            piexif.GPSIFD.GPSLongitudeRef: b"E",
            piexif.GPSIFD.GPSLongitude: ((121, 1), (28, 1), (7890, 100)),
        },
        "1st": {},
        "thumbnail": None,
    }
    gps_jpg = ROOT / "gps.jpg"
    image.save(gps_jpg, "JPEG", quality=90, exif=piexif.dump(gps_exif))

    orientation_exif = {
        "0th": {piexif.ImageIFD.Orientation: 6},
        "Exif": {},
        "GPS": {},
        "1st": {},
        "thumbnail": None,
    }
    orientation_jpg = ROOT / "orientation-6.jpg"
    image.save(
        orientation_jpg,
        "JPEG",
        quality=90,
        exif=piexif.dump(orientation_exif),
    )

    fake_jpg = ROOT / "fake-extension.jpg"
    image.save(fake_jpg, "PNG", optimize=True)

    oversized_6mb = ROOT / "oversized-6mb.jpg"
    pad_to_size(normal_jpg, oversized_6mb, 6 * 1024 * 1024)

    oversized_51mb = ROOT / "oversized-51mb.jpg"
    pad_to_size(normal_jpg, oversized_51mb, 51 * 1024 * 1024)

    for path in sorted(ROOT.iterdir()):
        print(f"{path.name}\t{path.stat().st_size}")


if __name__ == "__main__":
    main()
