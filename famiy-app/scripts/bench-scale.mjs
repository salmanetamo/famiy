// Scale benchmark for the graph screen. Serves a synthetic family of N people to the PRODUCTION build
// (vite preview) with the whole API mocked, so it needs no database, no account and no API process,
// and measures what the browser does with it: load, long tasks, zoom and pan frame rate, selecting a
// person, searching, and the cost of one write (refetch + new layout + redraw).
//
//   npm run bench:scale                       builds, then runs 1,000 / 2,000 / 5,000 / 10,000 x 3 runs
//   node scripts/bench-scale.mjs --sizes 1000,5000 --runs 1
//   node scripts/bench-scale.mjs --headed     a visible window, so a real GPU is used (run it on the
//                                             machine whose speed you care about)
//   node scripts/bench-scale.mjs --engine chromium   the full Chromium in headless mode (can use the GPU)
//   node scripts/bench-scale.mjs --out results.json
//
// What it does NOT measure: the real API (response time for GET /graph at that size), a phone, or
// anything about the people who use it. The default engine is Playwright's headless shell, which
// renders in software, so its numbers are pessimistic for paint-heavy work.
import { chromium } from '@playwright/test';
import { existsSync, writeFileSync } from 'node:fs';
import { argv, exit } from 'node:process';
import { preview } from 'vite';

const args = Object.fromEntries(
  argv
    .slice(2)
    .map((a) => a.replace(/^--/, '').split('='))
    .map(([k, v]) => [k, v ?? 'true']),
);
const sizes = (args.sizes ?? '1000,2000,5000,10000').split(',').map(Number);
const runs = Number(args.runs ?? 3);
const headed = args.headed === 'true';
const engine = args.engine ?? 'shell';
const LOAD_TIMEOUT = Number(args.timeout ?? 600_000);

/** A deterministic family of `count` people: couples with 1 to 3 children, most children partnered. */
function synthetic(count) {
  let seed = 7;
  const random = () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296;
  const persons = [];
  const links = [];
  const partnerships = [];
  const make = (year) => {
    const id = persons.length + 1;
    persons.push({
      id,
      given_name: `Person ${String(id)}`,
      family_name: null,
      birth_date: `${String(year)}-01-01`,
      birth_approx: true,
      death_date: null,
    });
    return { id, year };
  };
  const queue = [];
  while (persons.length < Math.max(2, count * 0.1)) {
    const a = make(1850 + Math.floor(random() * 10));
    const b = make(a.year + 2);
    partnerships.push({ partners: [a.id, b.id], kind: 'marriage' });
    queue.push([a, b]);
  }
  while (persons.length < count && queue.length) {
    const [a, b] = queue.shift();
    const kids = 1 + Math.floor(random() * 3);
    for (let i = 0; i < kids && persons.length < count; i++) {
      const c = make(Math.max(a.year, b.year) + 25 + i * 2);
      links.push({ parent_id: a.id, child_id: c.id }, { parent_id: b.id, child_id: c.id });
      if (persons.length < count && random() < 0.8) {
        const s = make(c.year + Math.floor(random() * 6) - 3);
        partnerships.push({ partners: [c.id, s.id], kind: 'marriage' });
        queue.push([c, s]);
      }
    }
  }
  return { persons, parent_links: links, partnerships };
}

const median = (values) => {
  const sorted = [...values].sort((x, y) => x - y);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
};

/** One wheel event per frame for 2 s, alternating in and out so the scale stays put. */
function zoomLoop(page) {
  return page.evaluate(
    () =>
      new Promise((resolve) => {
        const viewport = document.querySelector('svg').parentElement;
        const gaps = [];
        const start = performance.now();
        let last = start;
        let frames = 0;
        const step = () => {
          const now = performance.now();
          gaps.push(now - last);
          last = now;
          viewport.dispatchEvent(
            new WheelEvent('wheel', {
              deltaY: frames % 2 ? 40 : -40,
              clientX: 640,
              clientY: 400,
              bubbles: true,
              cancelable: true,
            }),
          );
          frames++;
          if (now - start < 2000) requestAnimationFrame(step);
          else resolve({ fps: (frames / (now - start)) * 1000, maxGap: Math.max(...gaps) });
        };
        requestAnimationFrame(step);
      }),
  );
}

async function measureOnce(browser, baseUrl, size) {
  const graphData = synthetic(size);
  const state = { graph: structuredClone(graphData), nextId: graphData.persons.length + 1 };
  const jsonBytes = Buffer.byteLength(JSON.stringify(graphData));

  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await ctx.addInitScript(() => {
    window.__long = [];
    try {
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) window.__long.push(entry.duration);
      }).observe({ type: 'longtask', buffered: true });
    } catch {
      /* long tasks unsupported */
    }
  });
  const page = await ctx.newPage();
  const problems = [];
  page.on('pageerror', (e) => problems.push(`pageerror: ${e.message}`));

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^\/api/, '');
    const method = route.request().method();
    const json = (status, body) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    if (path === '/auth/me')
      return json(200, { id: 1, email: 'bench@famiy.local', created_at: '2026-01-01T00:00:00Z' });
    if (path === '/graph') return json(200, state.graph);
    if (method === 'POST' && path === '/persons') {
      const input = JSON.parse(route.request().postData() ?? '{}');
      const person = {
        id: state.nextId++,
        given_name: input.given_name,
        family_name: input.family_name ?? null,
        birth_date: input.birth_date ?? null,
        birth_approx: false,
        death_date: null,
      };
      state.graph.persons.push(person);
      return json(201, {
        ...person,
        bio: null,
        residence: null,
        created_at: '2026-01-01T00:00:00Z',
      });
    }
    const one = /^\/persons\/(\d+)$/.exec(path);
    if (method === 'GET' && one) {
      const person = state.graph.persons.find((p) => p.id === Number(one[1]));
      return person
        ? json(200, { ...person, bio: null, residence: null, created_at: '2026-01-01T00:00:00Z' })
        : json(404, { error: { code: 'person_not_found', message: 'no' } });
    }
    return json(404, { error: { code: 'not_found', message: path } });
  });

  const result = { size, persons: graphData.persons.length, jsonKB: Math.round(jsonBytes / 1024) };

  // load: navigation -> data shown as "Laying out" -> worker layout + render -> first card drawn
  const t0 = Date.now();
  await page.goto(baseUrl, { waitUntil: 'commit' });
  await page.waitForSelector('text=Laying out your family', { timeout: LOAD_TIMEOUT });
  const tLaying = Date.now();
  await page.waitForSelector('[data-person-id]', { timeout: LOAD_TIMEOUT });
  const tFirst = Date.now();
  // Only the people near the view are drawn (viewport culling), so "every card" is no longer a thing
  // to wait for: the first card is the end of the load. Give it a moment to settle.
  const tAll = tFirst;
  await page.waitForTimeout(800);
  result.cardsDrawn = await page.locator('[data-person-id]').count();
  result.toLayoutStartMs = tLaying - t0;
  result.layoutAndDrawMs = tFirst - tLaying;
  result.totalLoadMs = tAll - t0;

  const longTasks = await page.evaluate(() => window.__long);
  result.longTaskCount = longTasks.length;
  result.longTaskMaxMs = Math.round(Math.max(0, ...longTasks));
  result.longTaskTotalMs = Math.round(longTasks.reduce((a, b) => a + b, 0));

  const cdp = await ctx.newCDPSession(page);
  await cdp.send('Performance.enable');
  const metrics = Object.fromEntries(
    (await cdp.send('Performance.getMetrics')).metrics.map((m) => [m.name, m.value]),
  );
  result.domElements = await page.evaluate(() => document.getElementsByTagName('*').length);
  result.jsHeapMB = Math.round(metrics.JSHeapUsedSize / 1048576);

  // selecting a person (mount the panel, restyle one card), median of 3, to the second paint
  const selectMs = [];
  for (const fraction of [0.3, 0.5, 0.7]) {
    selectMs.push(
      await page.evaluate(async (f) => {
        const cards = document.querySelectorAll('[data-person-id]');
        const card = cards[Math.floor(cards.length * f)];
        const start = performance.now();
        card.dispatchEvent(new MouseEvent('click', { bubbles: true }));
        await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
        return performance.now() - start;
      }, fraction),
    );
  }
  result.selectMs = Math.round(median(selectMs));

  // typing in the search box: time until the list shows
  const box = page.getByRole('combobox', { name: 'Find a person' });
  const searchMs = [];
  for (const query of ['Person 1', 'Person 2', 'Person 33']) {
    await box.fill('');
    const start = Date.now();
    await box.fill(query);
    await page.waitForSelector('[role=option]', { timeout: 60_000 });
    searchMs.push(Date.now() - start);
  }
  result.searchMs = Math.round(median(searchMs));
  await box.fill('');
  await page.keyboard.press('Escape');

  // zoom at the view the app opens with, then again after "Fit" (the whole tree in view)
  const zoom = await zoomLoop(page);
  result.zoomFps = Math.round(zoom.fps);
  result.zoomMaxFrameMs = Math.round(zoom.maxGap);
  await page.getByRole('button', { name: 'Fit' }).click();
  await page.waitForTimeout(800);
  const zoomFit = await zoomLoop(page);
  result.zoomAtFitFps = Math.round(zoomFit.fps);
  result.zoomAtFitMaxFrameMs = Math.round(zoomFit.maxGap);

  // pan: a real mouse drag of 150 moves; the page records the frames it manages while it lasts
  await page.evaluate(() => {
    window.__frames = [];
    window.__recording = true;
    const tick = (now) => {
      window.__frames.push(now);
      if (window.__recording) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  await page.mouse.move(300, 500);
  await page.mouse.down();
  for (let i = 1; i <= 150; i++) await page.mouse.move(300 + i * 2, 500 + i);
  await page.mouse.up();
  const frames = await page.evaluate(() => {
    window.__recording = false;
    return window.__frames;
  });
  const gaps = frames.slice(1).map((t, i) => t - frames[i]);
  result.panFps = Math.round((gaps.length / ((frames.at(-1) - frames[0]) / 1000)) * 10) / 10;
  result.panMaxFrameMs = Math.round(Math.max(0, ...gaps));

  // one write: add a person, then the refetch, the new layout and the redraw, until the new card has focus
  await page.getByRole('button', { name: 'Add person' }).first().click();
  const form = page.getByRole('region', { name: 'Add a person' });
  await form.getByLabel(/Given name/).fill('Bench');
  const newId = state.nextId;
  const tWrite = Date.now();
  await form.getByRole('button', { name: 'Add person' }).click();
  try {
    await page.waitForFunction(
      (id) => document.activeElement?.getAttribute('data-person-id') === String(id),
      newId,
      { timeout: LOAD_TIMEOUT },
    );
  } catch {
    // say what the page looked like, so a stuck write can be understood and not just timed out
    const report = await page.evaluate(
      (id) => ({
        focused: document.activeElement?.outerHTML.slice(0, 160),
        newCardInDom: document.querySelector(`[data-person-id="${String(id)}"]`) !== null,
        cardsDrawn: document.querySelectorAll('[data-person-id]').length,
        status: [...document.querySelectorAll('[role=status]')].map((e) => e.textContent),
        alerts: [...document.querySelectorAll('[role=alert]')].map((e) => e.textContent),
        formStillOpen: document.querySelector('form') !== null,
        view: document.querySelector('svg g')?.style.transform,
      }),
      newId,
    );
    throw new Error(`the new person never got focus: ${JSON.stringify(report)}`);
  }
  result.writeToFocusedMs = Date.now() - tWrite;

  result.problems = problems.length;
  await ctx.close();
  return result;
}

const METRICS = [
  ['totalLoadMs', 'load, whole (ms)'],
  ['layoutAndDrawMs', '  layout + first draw (ms)'],
  ['cardsDrawn', 'people drawn after load'],
  ['longTaskMaxMs', 'longest long task (ms)'],
  ['longTaskTotalMs', 'long tasks, total (ms)'],
  ['domElements', 'DOM elements'],
  ['jsHeapMB', 'JS heap, main thread (MB)'],
  ['selectMs', 'select a person (ms)'],
  ['searchMs', 'search: type -> list (ms)'],
  ['zoomFps', 'zoom, frames per second'],
  ['zoomMaxFrameMs', 'zoom, longest frame (ms)'],
  ['zoomAtFitFps', 'zoom after Fit, frames per second'],
  ['zoomAtFitMaxFrameMs', 'zoom after Fit, longest frame (ms)'],
  ['panFps', 'pan (real drag), frames per second'],
  ['panMaxFrameMs', 'pan, longest frame (ms)'],
  ['writeToFocusedMs', 'one write -> new card focused (ms)'],
];

if (!existsSync('dist/index.html')) {
  console.error(
    'No build found. Run `npm run build` first (or use `npm run bench:scale`, which does).',
  );
  exit(1);
}
const server = await preview({
  preview: { port: 4173, strictPort: false, open: false },
  logLevel: 'error',
});
const baseUrl = server.resolvedUrls.local[0];
const launchOptions = { headless: !headed };
if (engine === 'chromium') launchOptions.channel = 'chromium';
const browser = await chromium.launch(launchOptions);

const probe = await browser.newPage();
const renderer = await probe.evaluate(() => {
  const gl = document.createElement('canvas').getContext('webgl');
  const info = gl?.getExtension('WEBGL_debug_renderer_info');
  return gl && info ? gl.getParameter(info.UNMASKED_RENDERER_WEBGL) : 'no WebGL';
});
await probe.close();
console.log(
  `engine: ${engine}${headed ? ' (headed)' : ' (headless)'}, browser ${browser.version()}, WebGL renderer: ${renderer}`,
);
console.log(`sizes ${sizes.join(', ')}, ${String(runs)} run(s) each, medians (min-max)\n`);

const all = [];
for (const size of sizes) {
  const results = [];
  for (let run = 1; run <= runs; run++) {
    try {
      results.push(await measureOnce(browser, baseUrl, size));
    } catch (error) {
      console.log(
        `  ${String(size)} people, run ${String(run)} FAILED: ${error.message.split('\n')[0]}`,
      );
      results.push({ size, error: error.message.split('\n')[0] });
      break; // a size that fails once is not retried: the bigger ones would only fail harder
    }
  }
  all.push({ size, results });
  const good = results.filter((r) => !r.error);
  if (good.length === 0) {
    console.log(`\n=== ${String(size)} people: no successful run ===\n`);
    break;
  }
  console.log(
    `\n=== ${String(size)} people (${String(good.length)} run(s); JSON ${String(good[0].jsonKB)} KB) ===`,
  );
  for (const [key, label] of METRICS) {
    const values = good.map((r) => r[key]).filter((v) => v !== undefined);
    console.log(
      `  ${label.padEnd(38)} ${String(median(values)).padStart(7)}   (${Math.min(...values)}-${Math.max(...values)})`,
    );
  }
  console.log(`  page errors: ${String(good.reduce((a, r) => a + r.problems, 0))}\n`);
}

if (args.out)
  writeFileSync(args.out, JSON.stringify({ engine, headed, renderer, runs, all }, null, 2));
await browser.close();
await server.close();
