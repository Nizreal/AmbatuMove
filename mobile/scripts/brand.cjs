// Original geometric AM monogram, rasterized directly without external assets.
const fs = require("fs");
const zlib = require("zlib");
function crc(b) {
  let c = 0xffffffff;
  for (const v of b) {
    c ^= v;
    for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (c & 1 ? 0xedb88320 : 0);
  }
  return (c ^ 0xffffffff) >>> 0;
}
function chunk(type, data) {
  const t = Buffer.from(type),
    len = Buffer.alloc(4),
    sum = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  sum.writeUInt32BE(crc(Buffer.concat([t, data])));
  return Buffer.concat([len, t, data, sum]);
}
const n = 512,
  raw = Buffer.alloc(n * (n * 4 + 1));
const segments = [
  [140, 350, 220, 160],
  [220, 160, 290, 350],
  [198, 275, 265, 275],
  [290, 350, 370, 160],
  [370, 160, 370, 350],
];
for (let y = 0; y < n; y++)
  for (let x = 0; x < n; x++) {
    let color = [41, 71, 182, 255];
    for (const [ax, ay, bx, by] of segments) {
      const t = Math.max(
        0,
        Math.min(
          1,
          ((x - ax) * (bx - ax) + (y - ay) * (by - ay)) /
            ((bx - ax) ** 2 + (by - ay) ** 2),
        ),
      );
      if (Math.hypot(x - ax - t * (bx - ax), y - ay - t * (by - ay)) < 17)
        color = [197, 234, 245, 255];
    }
    const o = y * (n * 4 + 1) + 1 + x * 4;
    raw.set(color, o);
  }
const header = Buffer.alloc(13);
header.writeUInt32BE(n);
header.writeUInt32BE(n, 4);
header[8] = 8;
header[9] = 6;
fs.writeFileSync(
  "assets/brand.png",
  Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk("IHDR", header),
    chunk("IDAT", zlib.deflateSync(raw)),
    chunk("IEND", Buffer.alloc(0)),
  ]),
);
