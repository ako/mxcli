//
// Cut a raw take into per-beat clips.
//
//   node cut-clips.js [capture/beats.json] [capture/clips.json]
//
// The whole file exists because of one measured fact: THE RECORDER'S CLOCK IS
// NOT THE VIDEO'S CLOCK, and it is wrong in two ways at once.
//
//   * an OFFSET — recording starts when the browser context is created, before
//     your first navigation has settled (take.js records this as `offset_s`);
//   * a SCALE — the capture drops frames while the page is busy, so the file
//     plays back LONGER than the wall-clock session it recorded. Measured at
//     ~1.065 on videos/sudoku-demo.
//
// Correcting only the offset is the trap: a constant that was right at the start
// was four seconds wrong by the end — the difference between cutting to the
// payoff screen and cutting to the screen before it. Three rounds of cuts showed
// the wrong moment in every beat before this was found, and each one looked
// plausible in isolation. So: two anchors, a linear map, and a contact sheet at
// the end because spot-checking two clips is exactly how it survived those three
// rounds.
//
// clips.json is the project's own edit list:
//
//   { "clips": [
//       { "id": "03-home",
//         "from": { "mark": "home",       "offset": -1.20 },
//         "to":   { "mark": "deal:click", "offset": -0.15 },
//         "target": 7.20,
//         "freeze": true }
//   ] }
//
// `target` is the finished clip length (voice duration + reading tail). `freeze`
// permits padding a SHORT clip by holding its final frame — legitimate on a
// static page, never to stretch an interaction, and always reported.
//

const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');

const FFMPEG = process.env.HYPERFRAMES_FFMPEG_PATH || 'ffmpeg';
const FFPROBE = process.env.HYPERFRAMES_FFPROBE_PATH || 'ffprobe';

const beatsFile = process.argv[2] || 'capture/beats.json';
const clipsFile = process.argv[3] || 'capture/clips.json';
const OUT = process.env.CLIPS_OUT || 'assets/clips';

// ffprobe missing is SILENT in more than one pipeline — it degrades rather than
// stopping. Check both binaries up front and treat a miss as fatal.
for (const bin of [FFMPEG, FFPROBE]) {
  try {
    execFileSync(bin, ['-version'], { stdio: 'ignore' });
  } catch {
    console.error(`FATAL: ${bin} not on PATH. Set HYPERFRAMES_FFMPEG_PATH / HYPERFRAMES_FFPROBE_PATH.`);
    process.exit(1);
  }
}

const meta = JSON.parse(fs.readFileSync(beatsFile, 'utf8'));
const spec = JSON.parse(fs.readFileSync(clipsFile, 'utf8'));
const src = path.join(path.dirname(beatsFile), 'raw', meta.video);
if (!fs.existsSync(src)) {
  console.error(`FATAL: raw take not found: ${src}`);
  process.exit(1);
}

const duration = (file) =>
  Number(execFileSync(FFPROBE, ['-v', 'error', '-show_entries', 'format=duration',
    '-of', 'csv=p=0', file]).toString().trim());

const rawDur = duration(src);
const markTime = (name) => {
  const b = meta.beats.find((x) => x.name === name);
  if (!b) throw new Error(`no such mark: '${name}' (have: ${meta.beats.map((x) => x.name).join(', ')})`);
  return b.t;
};
const lastMark = Math.max(...meta.beats.map((b) => b.t));

// The two anchors. A is where mark 0 sits in the file; B stretches wall-clock
// marks onto file time.
const A = typeof meta.offset_s === 'number' ? meta.offset_s : 0;
const B = (rawDur - A) / (lastMark + (meta.post_roll_s ?? 1.0));

console.log(`  raw take ${rawDur.toFixed(2)}s, marks span ${lastMark.toFixed(2)}s`);
console.log(`  video_t = ${A.toFixed(2)} + ${B.toFixed(4)} x mark_t`);

// Reject rather than produce a plausible-looking wrong cut.
if (A < 0 || A > 30) throw new Error(`implausible start offset ${A}s — check the take`);
if (B < 0.95 || B > 1.25) throw new Error(`implausible clock scale ${B.toFixed(4)} — check the take`);

const at = (point) => A + B * (markTime(point.mark) + (point.offset || 0));

fs.mkdirSync(OUT, { recursive: true });
const report = [];

for (const c of spec.clips) {
  const from = at(c.from);
  const avail = at(c.to) - from;
  if (avail <= 0) throw new Error(`${c.id}: '${c.to.mark}' is not after '${c.from.mark}'`);

  const target = c.target ?? avail;
  const short = target - avail;
  if (short > 0.05 && !c.freeze) {
    throw new Error(
      `${c.id}: only ${avail.toFixed(2)}s of take for a ${target.toFixed(2)}s clip. ` +
      `Set "freeze": true ONLY if this beat ends on a static screen; otherwise re-record it longer.`);
  }

  const filters = ['fps=30'];
  if (c.trimBottom) filters.unshift(`crop=in_w:in_h-${c.trimBottom}:0:0`);
  // tpad holds the final frame; it is a no-op when the clip is long enough.
  if (short > 0.05) filters.unshift(`tpad=stop_mode=clone:stop_duration=${short.toFixed(3)}`);

  const dst = path.join(OUT, `${c.id}.mp4`);
  execFileSync(FFMPEG, [
    '-y', '-loglevel', 'error', '-ss', from.toFixed(3), '-i', src,
    '-vf', filters.join(','), '-t', target.toFixed(3),
    '-an', '-c:v', 'libx264', '-preset', 'slow', '-crf', '18',
    '-pix_fmt', 'yuv420p', '-movflags', '+faststart', dst,
  ], { stdio: ['ignore', 'inherit', 'inherit'] });

  const out = duration(dst);
  report.push({
    id: c.id, from: +from.toFixed(2), source_s: +avail.toFixed(2),
    frozen_s: short > 0.05 ? +short.toFixed(2) : 0, out_s: +out.toFixed(2),
  });
  console.log(`  ${c.id.padEnd(16)} src ${from.toFixed(2)}s +${avail.toFixed(2)}s` +
    `${short > 0.05 ? `  (+${short.toFixed(2)}s frozen)` : ''}  ->  ${out.toFixed(2)}s`);
}

// Verify BY LOOKING. One frame from the middle of every clip, tiled — the check
// that would have caught the wrong offset on the first round instead of the
// fourth. Look at it before assembling: each tile must show its own beat.
const sheetDir = path.join(path.dirname(beatsFile), 'sheet');
fs.rmSync(sheetDir, { recursive: true, force: true });
fs.mkdirSync(sheetDir, { recursive: true });
for (const r of report) {
  execFileSync(FFMPEG, ['-y', '-loglevel', 'error',
    '-ss', (r.out_s / 2).toFixed(2), '-i', path.join(OUT, `${r.id}.mp4`),
    '-frames:v', '1', '-vf', 'scale=480:-1', path.join(sheetDir, `${r.id}.jpg`)],
    { stdio: ['ignore', 'inherit', 'inherit'] });
}
const sheet = path.join(path.dirname(beatsFile), 'contact-sheet.jpg');
// Fit the grid to the clip count. A fixed tile size leaves most of the sheet
// black for a short film, which makes the thing you are supposed to study
// harder to read.
const cols = Math.min(3, report.length);
const rows = Math.ceil(report.length / cols);
execFileSync(FFMPEG, ['-y', '-loglevel', 'error', '-pattern_type', 'glob',
  '-i', path.join(sheetDir, '*.jpg'), '-vf', `tile=${cols}x${rows}`, '-frames:v', '1', sheet],
  { stdio: ['ignore', 'inherit', 'inherit'] });

fs.writeFileSync(path.join(path.dirname(beatsFile), 'clips-report.json'),
  JSON.stringify({ source: meta.video, offset_s: A, clock_scale: +B.toFixed(4), clips: report }, null, 1));

console.log(`\n  ${report.length} clips -> ${OUT}`);
const frozen = report.filter((r) => r.frozen_s);
if (frozen.length) console.log(`  frozen tails: ${frozen.map((r) => `${r.id} +${r.frozen_s}s`).join(', ')}`);
console.log(`  contact sheet -> ${sheet}   <- LOOK AT THIS before assembling`);
