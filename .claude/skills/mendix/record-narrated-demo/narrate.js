//
// The narration overlay. Injected into the page under test; asserts nothing.
//
// Three jobs, and only the first is obvious:
//
//   1. Show a caption a viewer can read while the app does something.
//   2. Keep something MOVING for the whole recording.
//   3. Look like the rest of the catalogue.
//
// (2) Playwright's video captures frames the compositor actually produces. A
// screen that is genuinely still during a reading pause can collapse to almost
// no video - the pause the viewer needed disappears, and the demo cuts from one
// action straight into the next. A small continuously animating element means
// idle time is recorded as idle time. That is what the sweeping hairline is
// for; it is not a spinner and it is not pretending anything is loading.
//
// (3) The capture is most of a Type A film's runtime, so this overlay is the
// film's furniture, not a debug HUD. It is built to
// `video-system/DESIGN-LANGUAGE.md` in ako/mxcli-intro-video - the palette, the
// flat plane, the grid and the caption band - because furniture that disagrees
// with the frames it cuts against reads as two designs. `checkOverlay()` below
// enforces the parts of that which are measurable in the page.
//

// The system palette (DESIGN-LANGUAGE.md §1). ONE accent, and it belongs to
// #demo-spot alone: the thing the viewer should be looking at. The caption
// plate is deliberately accent-free - two teal things competing is the single
// most common way a frame stops working.
//
// A product demonstration may swap `accent` (and `ground`) for the demonstrated
// app's own - see types/product-demonstration.md, "Type and accent". That is
// the one documented allowance; pass it through `configure()`, do not edit it
// here.
const SYSTEM = {
    ground: '#0e1116',   // full-bleed ground
    line: '#262828',   // 1px hairline - carries all structure
    ink: '#e6edf3',   // primary text, 16.0:1
    faint: '#6b7787',   // chrome labels, 4.15:1 - floor for 24px+ mono only
    accent: '#3fbdb8',   // the only colour
};

// Geometry is stated in VIDEO pixels (the 1920x1080 frame), never in CSS
// pixels, and converted with `zoom`. This is not pedantry: take.js reaches a
// fixed-width layout with CSS `zoom`, so a plate declared as `96px` tall lands
// at 96*zoom in the file - at the zoom sudoku-demo used, 162 device px, which
// is neither the caption band nor anything else in the system. Tell the overlay
// the zoom and every number below is the number that reaches the video.
const FRAME = { width: 1920, height: 1080 };
const BAND = 184;   // bottom 17% of 1080 - the caption band (DESIGN-LANGUAGE.md §3)
const GUTTER = 96;    // safe-area x - the caption's left edge aligns with the grid
const CAPTION_PX = 34;
const LABEL_PX = 26;    // >= 24px, the floor for `faint` on mono

let cfg = {
    ...SYSTEM,
    zoom: 1,
    // Set to a family that is actually loaded in the page. Left null, the plate
    // inherits the app's own computed body font, which is both the seam-free
    // choice for a Type A film and the only one that cannot fall back: a plate
    // asking for 'Segoe UI' renders in DejaVu Sans on a clean build machine and
    // nothing says so.
    font: null,
    // checkOverlay() throws rather than warns. The pipeline's default failure
    // mode is to keep going and hand you a plausible-looking film.
    strict: true,
};

/** Override the tokens, the zoom, or the caption font. Call before install(). */
function configure(opts = {}) {
    cfg = { ...cfg, ...opts };
    return cfg;
}

// Marks that are not in the shipped fonts and render as tofu. The system
// derives this set from the film font's cmap (PRODUCTION.md §3) - `·`, `—` and
// `–` ARE present in Recursive and are not banned; `≠`, `→`, `✓` and `✗` are
// not present anywhere in the catalogue's fonts. A caption is set in the APP's
// font, whose coverage is its own business, so this list is the floor rather
// than the whole check: when you know the file, derive the set from it.
const BANNED_GLYPHS = /[≠→←↑↓⇒⇐➔➡✓✗✔✘≤≥]/;

const css = () => {
    const z = cfg.zoom || 1;
    const px = (v) => `${(v / z).toFixed(3)}px`;
    const viewportW = FRAME.width / z;
    return `
  /* pointer-events: none for the same reason #demo-spot has it, and the reason
     is easy to miss here: the plate is a full-width bar pinned to the BOTTOM of
     the viewport, which is exactly where Mendix puts a page footer's buttons.
     Without this, any control the plate covers becomes unclickable and
     Playwright retries for 30s against "div.text from div#demo-narration
     subtree intercepts pointer events" before failing the take — a failure that
     names the overlay but not the reason. Reported by ako/ChipCoV1, where it
     killed the take on an Approve button. The caption is read, never clicked,
     so it gives up pointer events for free. */
  #demo-narration {
    position: fixed; left: 0; right: 0; bottom: 0; z-index: 2147483647;
    box-sizing: border-box; height: ${px(BAND)};
    pointer-events: none; overflow: hidden;
    display: flex; align-items: center; gap: ${px(28)};
    padding: 0 ${px(GUTTER)};
    background: ${cfg.ground}; color: ${cfg.ink};
    /* Depth is a hairline and the value step to the app's own ground. NO
       box-shadow: the flat plane is not a preference, it is what stops the
       film reading as a template (DESIGN-LANGUAGE.md §4). */
    border-top: ${px(1)} solid ${cfg.line};
    font-size: ${px(CAPTION_PX)}; line-height: 1.35; font-weight: 500;
    /* A code ligature composes "!=" into a single glyph: the bytes stay "!="
       and the picture shows a different character. On a caption quoting what
       the app said, that is a claim rather than a typographic choice. */
    font-variant-ligatures: none;
    font-feature-settings: "liga" 0, "clig" 0, "calt" 0, "dlig" 0;
    transform: translateY(100%);
    transition: transform .45s cubic-bezier(.16,.84,.44,1);
  }
  #demo-narration.up { transform: translateY(0); }

  /* The compositor keep-alive, and the reason it is a hairline rather than the
     spinning ring this used to be: the system has no rounded corners, so a ring
     could only be a spinning square. A segment travelling the plate's top edge
     is the same guarantee in the system's own vocabulary — 1px structure, in
     the "faint" role, never the accent — and it doubles as a read-progress cue.
     Continuous, not per-caption: a hold between captions still has to produce
     frames. */
  #demo-narration .sweep {
    position: absolute; top: 0; left: 0;
    width: ${px(180)}; height: ${px(2)};
    background: ${cfg.faint};
    animation: demo-sweep 2.6s linear infinite;
  }
  @keyframes demo-sweep {
    from { transform: translateX(${px(-180)}); }
    to   { transform: translateX(${viewportW.toFixed(2)}px); }
  }

  #demo-narration .text { flex: 1 1 auto; }
  #demo-narration .step {
    flex: 0 0 auto; font-family: ui-monospace, monospace;
    font-size: ${px(LABEL_PX)}; letter-spacing: .14em;
    text-transform: uppercase; color: ${cfg.faint};
  }

  /* Where the viewer should be looking, and the film's one accent event.
     Drawn, not clicked — a real click ring would move the cursor and the page
     under it. Square, because the plane is flat; the pulse is opacity, because
     a soft expanding glow is banned for the same reason the shadow is. */
  #demo-spot {
    position: fixed; z-index: 2147483646; pointer-events: none;
    border: ${px(3)} solid ${cfg.accent};
    transition: left .4s cubic-bezier(.16,.84,.44,1),
                top .4s cubic-bezier(.16,.84,.44,1),
                opacity .4s linear;
    opacity: 0;
  }
  #demo-spot.on { opacity: 1; animation: demo-pulse 1.6s ease-in-out infinite; }
  @keyframes demo-pulse { 0%, 100% { opacity: 1; } 50% { opacity: .42; } }

  /* Reserve the plate's own height at the foot of the page. pointer-events
     above makes a covered control CLICKABLE; this makes it VISIBLE, and the
     film needs both — a click that lands under an opaque caption is a beat the
     viewer cannot see happen, which is the same dead beat by another route.
     Applied once at install, before the take starts, so nothing shifts mid-shot. */
  :root { --demo-plate-height: ${px(BAND)}; }
  body { padding-bottom: var(--demo-plate-height) !important; }
`;
};

/** Put the overlay on the page. Safe to call again after a navigation. */
async function install(page, opts) {
    if (opts) configure(opts);
    await page.addStyleTag({ content: css() }).catch(() => {});
    await page.evaluate((font) => {
        let bar = document.getElementById('demo-narration');
        if (!bar) {
            bar = document.createElement('div');
            bar.id = 'demo-narration';
            bar.innerHTML = '<div class="sweep"></div><div class="text"></div><div class="step"></div>';
            document.body.appendChild(bar);
            const spot = document.createElement('div');
            spot.id = 'demo-spot';
            document.body.appendChild(spot);
        }
        // Inherit the app's own typeface unless told otherwise. Furniture set in
        // a different face from the UI it wraps reads as a seam, and a family
        // this file names is a family that can be missing.
        bar.style.fontFamily = font || getComputedStyle(document.body).fontFamily;
    }, cfg.font).catch(() => {});
    return checkOverlay(page);
}

/**
 * The design rules that are measurable in the page, as a check that fails the
 * take rather than the render. Everything here is a rule from
 * DESIGN-LANGUAGE.md; the geometry ones exist because a wrong `zoom` puts a
 * perfectly styled plate in the wrong place and nothing looks wrong until the
 * capture is cut against a composed frame.
 *
 * Returns { ok, problems }. Throws in strict mode (the default).
 */
async function checkOverlay(page) {
    const rgb = (hex) => {
        const h = hex.replace('#', '');
        return `rgb(${parseInt(h.slice(0, 2), 16)}, ${parseInt(h.slice(2, 4), 16)}, ${parseInt(h.slice(4, 6), 16)})`;
    };
    const problems = await page.evaluate(([z, want, band, frameH]) => {
        const out = [];
        const bar = document.getElementById('demo-narration');
        const spot = document.getElementById('demo-spot');
        if (!bar || !spot) return ['overlay is not installed'];
        const s = getComputedStyle(bar);

        if (s.boxShadow !== 'none') out.push(`caption plate has a box-shadow (${s.boxShadow}) — the plane is flat`);
        for (const c of ['borderTopLeftRadius', 'borderTopRightRadius']) {
            if (parseFloat(s[c]) > 0.5) out.push(`caption plate has a corner radius (${s[c]}) — the plane is flat`);
        }
        if (parseFloat(getComputedStyle(spot).borderTopLeftRadius) > 0.5) {
            out.push('the pointer has a corner radius — the plane is flat');
        }
        if (s.backgroundColor !== want.ground) {
            out.push(`caption plate ground is ${s.backgroundColor}, expected ${want.ground}`);
        }
        if (/system-ui|Segoe UI|-apple-system|BlinkMacSystem/.test(s.fontFamily)) {
            out.push(`caption font resolves to a system stack (${s.fontFamily}) — it will not exist on a clean build machine`);
        }
        // Geometry, in VIDEO pixels. The two coordinate spaces differ and it
        // is measured, not assumed: under `html{zoom}` Chromium reports
        // getBoundingClientRect() ALREADY zoom-adjusted (a 109.25px-tall plate
        // at zoom 1.6842 measures 183.98) while getComputedStyle() still
        // reports CSS px (109.25). So the rect is compared raw and the computed
        // padding is multiplied. Getting this backwards is how a perfectly
        // styled plate ends up 310px tall in the file.
        // Measure where the plate SITS, not where it happens to be sliding
        // through: install() runs both before the first raise (parked a full
        // height below the fold) and again mid-take, where the 0.45s entrance
        // transition may still be in flight — which read as a 5px band error
        // once. Neutralising the transform and restoring it inside one
        // synchronous block never reaches a paint, so nothing flashes.
        const t0 = bar.style.transform, n0 = bar.style.transition;
        bar.style.transition = 'none';
        bar.style.transform = 'translateY(0)';
        const r = bar.getBoundingClientRect();
        bar.style.transform = t0;
        bar.style.transition = n0;
        const height = Math.round(r.height);
        const top = Math.round(r.top);
        if (Math.abs(height - band) > 3) out.push(`caption plate is ${height} video px tall, the band is ${band} — check \`zoom\``);
        if (Math.abs(top - (frameH - band)) > 3) out.push(`caption plate top is y${top}, the band starts at y${frameH - band}`);
        const pad = parseFloat(getComputedStyle(document.body).paddingBottom) || 0;
        if (Math.round(pad * z) < band - 3) out.push(`body reserves only ${Math.round(pad * z)} video px for a ${band}px plate — the plate will cover a control`);
        // One accent event: it belongs to the pointer, and to nothing else.
        if (JSON.stringify([s.color, s.backgroundColor, s.borderTopColor]).includes(want.accent)) {
            out.push('the caption plate carries the accent — one accent event per frame, and it is the pointer');
        }
        return out;
    }, [cfg.zoom || 1, { ground: rgb(cfg.ground), accent: rgb(cfg.accent) }, BAND, FRAME.height]).catch((e) => [`overlay check failed: ${e.message}`]);

    if (problems.length) {
        const msg = `narrate.js: overlay does not conform:\n  - ${problems.join('\n  - ')}`;
        if (cfg.strict) throw new Error(msg);
        console.warn(msg);
    }
    return { ok: problems.length === 0, problems };
}

/**
 * Show a caption and hold it long enough to be read.
 *
 * The hold is derived from the length of the sentence, not from a fixed number:
 * a demo that gives every caption the same 2 seconds either rushes the long ones
 * or stalls on the short ones. The 2.5s floor is the product-demonstration
 * type's, and the hold is a FLOOR in the other direction too — when the screen
 * takes longer to read than the caption, or when a narration line is longer
 * than both, pass the measured duration as `holdMs`.
 */
async function say(page, text, stepLabel, opts = {}) {
    const bad = text.match(BANNED_GLYPHS);
    if (bad) {
        throw new Error(
            `narrate.js: caption contains ${JSON.stringify(bad[0])}, which renders as tofu ` +
            `in the catalogue's fonts. Use ASCII ("->", "!=") or an inline SVG mark.\n  ${text}`);
    }
    await install(page);
    await page.evaluate(([t, s]) => {
        const bar = document.getElementById('demo-narration');
        if (!bar) return;
        bar.querySelector('.text').textContent = t;
        bar.querySelector('.step').textContent = s || '';
        bar.classList.add('up');
    }, [text, stepLabel]);

    const words = text.split(/\s+/).length;
    const readMs = opts.holdMs || Math.max(2500, Math.round(words * 280));
    await page.waitForTimeout(readMs);
}

/**
 * Scroll a target into view, including SIDEWAYS inside a scrolling container.
 *
 * On a phone the planning grid is wider than the screen and its action button
 * sits past the right edge - a real user swipes the table across to reach it, so
 * the demo does the same. `scrollIntoViewIfNeeded` only handles the vertical
 * case here; `inline: 'center'` is what moves a horizontally scrolled container.
 *
 * This is not the demo working around the app. If the button were genuinely
 * unreachable, the walk would stop here and that would be the finding.
 */
async function bringIntoView(page, selector) {
    await page.evaluate((sel) => {
        const el = document.querySelector(sel);
        if (el) el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'smooth' });
    }, selector).catch(() => {});
    await page.waitForTimeout(900);
}

/**
 * Draw attention to an element without touching it.
 *
 * The two coordinate spaces meet here and they do NOT agree: under
 * `html{zoom}` Chromium reports getBoundingClientRect() in VIDEO pixels, while
 * a style value is read back as CSS pixels and zoomed on the way out. Assigning
 * the rect straight across therefore multiplies the position by the zoom — at
 * 1.6842 a target at (168,202) was ringed at (274,330), which is a highlight
 * sitting on the wrong control, or off the screen entirely, with nothing in the
 * take saying so. Divide, then pad in video pixels like every other number.
 */
async function point(page, selector) {
    await install(page);
    await page.evaluate(([sel, z]) => {
        const spot = document.getElementById('demo-spot');
        const el = document.querySelector(sel);
        if (!spot || !el) return;
        const r = el.getBoundingClientRect();
        const pad = 10;                       // video px of air around the target
        spot.style.left = ((r.left - pad) / z) + 'px';
        spot.style.top = ((r.top - pad) / z) + 'px';
        spot.style.width = ((r.width + pad * 2) / z) + 'px';
        spot.style.height = ((r.height + pad * 2) / z) + 'px';
        spot.classList.add('on');
    }, [selector, cfg.zoom || 1]).catch(() => {});
    await page.waitForTimeout(700);
}

async function unpoint(page) {
    await page.evaluate(() => {
        const spot = document.getElementById('demo-spot');
        if (spot) spot.classList.remove('on');
    }).catch(() => {});
}

/**
 * A click a viewer can follow.
 *
 * Scroll it into view, mark it, wait a beat, then click. A cursor that arrives
 * and clicks in the same frame reads as a glitch rather than as an action.
 */
async function clickSlowly(page, selector, pauseMs = 900) {
    const el = page.locator(selector).first();
    await bringIntoView(page, selector);
    await point(page, selector);
    await page.waitForTimeout(pauseMs);
    await el.click();
    await unpoint(page);
}

/** Type at a speed a viewer can follow, then commit the field. */
async function typeSlowly(page, selector, value, perKeyMs = 140) {
    const el = page.locator(selector).first();
    await bringIntoView(page, selector);
    await point(page, selector);
    await el.focus();
    await page.keyboard.press('Control+a');
    await page.keyboard.type(value, { delay: perKeyMs });
    await page.waitForTimeout(600);
    await page.keyboard.press('Tab');
    await unpoint(page);
}

module.exports = {
    configure, install, checkOverlay, say, point, unpoint,
    bringIntoView, clickSlowly, typeSlowly,
    SYSTEM, FRAME, BAND, BANNED_GLYPHS,
};
