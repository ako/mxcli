//
// The narration overlay. Injected into the page under test; asserts nothing.
//
// Two jobs, and the second one is not decoration:
//
//   1. Show a caption a viewer can read while the app does something.
//   2. Keep something MOVING for the whole recording.
//
// Playwright's video captures frames the compositor actually produces. A screen
// that is genuinely still during a reading pause can collapse to almost no
// video - the pause the viewer needed disappears, and the demo cuts from one
// action straight into the next. A small continuously animating element means
// idle time is recorded as idle time. That is what the progress ring is for; it
// is not a spinner and it is not pretending anything is loading.
//

const OVERLAY_CSS = `
  #demo-narration {
    position: fixed; left: 0; right: 0; bottom: 0; z-index: 2147483647;
    display: flex; align-items: center; gap: 14px;
    padding: 16px 22px;
    background: rgba(17, 24, 39, .94);
    color: #fff;
    font: 500 17px/1.45 system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
    box-shadow: 0 -8px 24px rgba(0,0,0,.22);
    transform: translateY(110%);
    transition: transform .45s cubic-bezier(.16,.84,.44,1);
  }
  #demo-narration.up { transform: translateY(0); }
  #demo-narration .ring {
    flex: 0 0 auto; width: 22px; height: 22px; border-radius: 50%;
    border: 2.5px solid rgba(255,255,255,.28); border-top-color: #fff;
    animation: demo-spin 1.15s linear infinite;
  }
  #demo-narration .text { flex: 1 1 auto; }
  #demo-narration .step {
    flex: 0 0 auto; font-size: 12.5px; letter-spacing: .06em;
    text-transform: uppercase; color: rgba(255,255,255,.55);
  }
  @keyframes demo-spin { to { transform: rotate(360deg); } }

  /* Where the viewer should be looking. Drawn, not clicked - a real click ring
     would move the cursor and the page under it. */
  #demo-spot {
    position: fixed; z-index: 2147483646; pointer-events: none;
    border-radius: 10px; border: 2.5px solid #2563eb;
    box-shadow: 0 0 0 4px rgba(37,99,235,.22);
    transition: all .4s cubic-bezier(.16,.84,.44,1);
    opacity: 0;
  }
  #demo-spot.on { opacity: 1; animation: demo-pulse 1.6s ease-in-out infinite; }
  @keyframes demo-pulse {
    0%, 100% { box-shadow: 0 0 0 4px rgba(37,99,235,.22); }
    50%      { box-shadow: 0 0 0 9px rgba(37,99,235,.10); }
  }
`;

/** Put the overlay on the page. Safe to call again after a navigation. */
async function install(page) {
    await page.addStyleTag({ content: OVERLAY_CSS }).catch(() => {});
    await page.evaluate(() => {
        if (document.getElementById('demo-narration')) return;
        const bar = document.createElement('div');
        bar.id = 'demo-narration';
        bar.innerHTML = '<div class="ring"></div><div class="text"></div><div class="step"></div>';
        document.body.appendChild(bar);
        const spot = document.createElement('div');
        spot.id = 'demo-spot';
        document.body.appendChild(spot);
    }).catch(() => {});
}

/**
 * Show a caption and hold it long enough to be read.
 *
 * The hold is derived from the length of the sentence, not from a fixed number:
 * a demo that gives every caption the same 2 seconds either rushes the long ones
 * or stalls on the short ones.
 */
async function say(page, text, stepLabel, opts = {}) {
    await install(page);
    await page.evaluate(([t, s]) => {
        const bar = document.getElementById('demo-narration');
        if (!bar) return;
        bar.querySelector('.text').textContent = t;
        bar.querySelector('.step').textContent = s || '';
        bar.classList.add('up');
    }, [text, stepLabel]);

    const words = text.split(/\s+/).length;
    const readMs = opts.holdMs || Math.max(2200, Math.round(words * 280));
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

/** Draw attention to an element without touching it. */
async function point(page, selector) {
    await install(page);
    await page.evaluate((sel) => {
        const spot = document.getElementById('demo-spot');
        const el = document.querySelector(sel);
        if (!spot || !el) return;
        const r = el.getBoundingClientRect();
        const pad = 6;
        spot.style.left = (r.left - pad) + 'px';
        spot.style.top = (r.top - pad) + 'px';
        spot.style.width = (r.width + pad * 2) + 'px';
        spot.style.height = (r.height + pad * 2) + 'px';
        spot.classList.add('on');
    }, selector).catch(() => {});
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

module.exports = { install, say, point, unpoint, bringIntoView, clickSlowly, typeSlowly };
