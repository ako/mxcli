//
// Recording integrity for a narrated demo. The companion to narrate.js:
// that file makes a take watchable, this one makes it TRUE — that the beat you
// meant to film actually happened, and that the timestamp you cut on points at
// it.
//
// Every guard here cost a take on ako/mxcli-intro-video (videos/sudoku-demo).
// None of it is theoretical, and none of it is a verdict about the app: a demo
// still never gates the build. These checks are about the RECORDING.
//
// Usage, from the per-project walkthrough script:
//
//   const { openTake } = require('./take.js');
//   const take = await openTake(browser, { url: 'http://127.0.0.1:8080/', zoom: 1.68 });
//   await take.goto();                       // navigates, settles, starts the clock
//   take.mark('home');
//   await take.click('.sd-key >> nth=0');    // paced, dialog-guarded
//   take.mark('entered');
//   await take.assertBeat('entered', () => page.locator('.sd-bad').count().then(n => n > 0),
//                         'the conflict must be flagged, or beat 07 shows nothing');
//   await take.finish();                     // closes context, writes beats.json
//

const fs = require('fs');
const path = require('path');

const DEFAULTS = {
  url: 'http://127.0.0.1:8080/',
  videoDir: 'capture/raw',
  beatsFile: 'capture/beats.json',
  // recordVideo.size PADS a smaller viewport into the video canvas — it does not
  // scale it — so a 1140x641 viewport recorded at 1920x1080 puts the app in the
  // top-left corner with grey around it. Keep viewport === size and reach the
  // layout you want with `zoom` instead (below).
  size: { width: 1920, height: 1080 },
  viewport: null,
  // CSS zoom keeps the pixels native: at zoom 1.6842 a 1920px viewport lays out
  // as 1140 CSS px (the width a fixed-width Mendix page wants) while Chromium
  // still rasterizes at full device resolution. Sharp and full-frame, where
  // shrinking the viewport is soft and letterboxed.
  zoom: 1,
  // Never drive the app faster than its runtime commits. Entering values back to
  // back made two Mendix microflows overlap and deadlock in Postgres; the
  // UpdateConflictException surfaced as a modal dialog that then swallowed every
  // later click and killed the take. Find the floor experimentally per app.
  minGap: 450,
  dialogSelector: '.mx-dialog-error',
  postRoll: 1.0,
  // finish() throws when an asserted beat did not hold. The pipeline's default
  // failure mode is to keep going and hand you a plausible-looking film with a
  // dead beat in it; fail loudly instead.
  strict: true,
};

async function openTake(browser, opts = {}) {
  const o = { ...DEFAULTS, ...opts };
  o.viewport = o.viewport || o.size;
  fs.mkdirSync(o.videoDir, { recursive: true });
  fs.mkdirSync(path.dirname(o.beatsFile), { recursive: true });

  // ANCHOR 1 of 2. Playwright starts recording when the context is created —
  // before your first navigation has even been issued, let alone settled. The
  // gap between this instant and the first mark is the offset the cut must
  // subtract. Capture it before newContext so it is never an underestimate.
  const videoT0 = Date.now();
  const context = await browser.newContext({
    viewport: o.viewport,
    recordVideo: { dir: o.videoDir, size: o.size },
  });
  const page = await context.newPage();

  const beats = [];
  const assertions = [];
  let dialogs = 0;
  let t0 = 0;
  let lastAction = 0;

  const mark = (name) => {
    const t = t0 ? (Date.now() - t0) / 1000 : 0;
    beats.push({ name, t: Number(t.toFixed(2)) });
    console.log(`  [${t.toFixed(2)}s] ${name}`);
    return t;
  };

  // A stylesheet does not survive a navigation, so this is re-applied after
  // every goto rather than set once.
  const applyZoom = async (z) => {
    const zoom = z || o.zoom;
    if (zoom && zoom !== 1) await page.addStyleTag({ content: `html{zoom:${zoom};}` });
  };

  // One runtime error must not cost the whole session: dismiss the dialog and
  // carry on, but count it — a take with dialogs in it needs looking at.
  const clearDialog = async () => {
    const dlg = page.locator(o.dialogSelector).first();
    // Visibility, not presence. Mendix ships the error-dialog container in the
    // DOM hidden, so a `count()` test fires on every single click: 800ms of
    // dead time each time and a dialog count that is pure noise.
    if (!(await dlg.isVisible().catch(() => false))) return false;
    dialogs++;
    console.log(`  !! runtime error dialog (${dialogs}) — dismissing`);
    const ok = dlg.locator('button').last();
    if (await ok.count()) await ok.click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(800);
    return true;
  };

  const pace = async () => {
    const since = Date.now() - lastAction;
    if (lastAction && since < o.minGap) await page.waitForTimeout(o.minGap - since);
    lastAction = Date.now();
  };

  // Paced + dialog-guarded wrappers. Use these rather than page.click directly:
  // the pacing is what keeps the runtime out of conflict, and the guard is what
  // stops one dialog from eating every later action.
  const click = async (selector, options) => {
    await pace();
    await clearDialog();
    await page.locator(selector).first().click({ timeout: 15000, ...options });
  };
  const type = async (selector, text, options) => {
    await pace();
    await clearDialog();
    await page.locator(selector).first().fill(text, { timeout: 15000, ...options });
  };

  // Check the state the beat is ABOUT, not that a click returned. A click can be
  // swallowed while the previous action's request is still in flight, and the
  // result looks like success: the sudoku board twice ended up full but not
  // solved, so the payoff never arrived and the control depending on it stayed
  // disabled. A beat that cannot be asserted is a beat you cannot trust.
  //
  // `probe` returns truthy for "the beat happened". It is given the page.
  const assertBeat = async (name, probe, why) => {
    let ok = false;
    let error = null;
    try {
      ok = !!(await probe(page));
    } catch (e) {
      error = e.message;
    }
    assertions.push({ beat: name, ok, why, error });
    if (ok) {
      console.log(`  ok   beat '${name}'`);
    } else {
      console.log(`  !!   beat '${name}' DID NOT HOLD — ${why}${error ? ` (${error})` : ''}`);
    }
    return ok;
  };

  const goto = async (url) => {
    await page.goto(url || o.url, { waitUntil: 'networkidle', timeout: 90000 });
    await applyZoom();
    await page.waitForTimeout(1500);
    // ANCHOR 2 of 2. The clock starts only once the first screen has settled, so
    // every mark is measured from a frame a viewer would recognise.
    t0 = Date.now();
    lastAction = 0;
  };

  const finish = async () => {
    mark('end');
    await page.waitForTimeout(o.postRoll * 1000);
    await context.close();          // flushes the video file

    const video = fs.readdirSync(o.videoDir).filter((f) => f.endsWith('.webm')).sort().pop();
    const failed = assertions.filter((a) => !a.ok);
    const meta = {
      video,
      viewport: o.viewport,
      video_size: o.size,
      zoom: o.zoom,
      dialogs,
      post_roll_s: o.postRoll,
      // The number the cut cannot be correct without.
      offset_s: Number(((t0 - videoT0) / 1000).toFixed(3)),
      beats,
      assertions,
    };
    fs.writeFileSync(o.beatsFile, JSON.stringify(meta, null, 1));

    console.log(`\n  video:  ${path.join(o.videoDir, video || '(none)')}`);
    console.log(`  beats:  ${beats.length} marks -> ${o.beatsFile}`);
    console.log(`  offset: ${meta.offset_s}s   dialogs: ${dialogs}`);
    if (failed.length) {
      console.log(`\n  ${failed.length} BEAT(S) DID NOT HOLD — this take is not usable as filmed:`);
      for (const f of failed) console.log(`    - ${f.beat}: ${f.why}`);
      if (o.strict) throw new Error(`${failed.length} beat(s) did not hold; re-record rather than cutting this take`);
    }
    return meta;
  };

  return { page, context, mark, assertBeat, goto, applyZoom, clearDialog, click, type, pace, finish };
}

module.exports = { openTake, DEFAULTS };
