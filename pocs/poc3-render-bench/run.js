'use strict';

/**
 * PoC-3 runner: drive bench.html in headless Chromium across tree sizes and
 * report render/interaction metrics.
 *
 * Caveat (printed in the report too): headless Chromium often uses software
 * rendering and may cap or free-run rAF differently from real hardware. Numbers
 * are indicative — use the SHAPE of the curve and the breakpoint, not absolute
 * FPS, and reconfirm a "keep SVG" call on a real device.
 */
const path = require('path');
const fs = require('fs');
const { chromium } = require('playwright');

const SIZES = [250, 500, 1000, 2000, 4000, 8000];

(async () => {
  const browser = await chromium.launch(); // headless
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  await page.goto('file://' + path.join(__dirname, 'bench.html'));
  await page.waitForFunction('window.__ready === true');

  const results = [];
  for (const n of SIZES) {
    // warm once, then measure each technique
    await page.evaluate((N) => window.runBench(N, 'viewbox'), n);
    const vb = await page.evaluate((N) => window.runBench(N, 'viewbox'), n);
    const tf = await page.evaluate((N) => window.runBench(N, 'transform'), n);
    results.push({ ...vb, fps_viewbox: vb.medianFps, dropped_viewbox: vb.droppedPct, fps_transform: tf.medianFps, dropped_transform: tf.droppedPct });
    if (n === 2000) await page.screenshot({ path: path.join(__dirname, 'out', 'sample-2000.png') });
    console.error(`  done ${n}`);
  }
  await browser.close();

  fs.writeFileSync(path.join(__dirname, 'out', 'results.json'), JSON.stringify(results, null, 2));

  const H = `${'persons'.padStart(8)}${'domEls'.padStart(9)}${'render(ms)'.padStart(11)}${'vbFPS'.padStart(8)}${'vbDrop%'.padStart(9)}${'tfFPS'.padStart(8)}${'tfDrop%'.padStart(9)}${'select(ms)'.padStart(12)}`;
  console.log('\nvb = viewBox pan/zoom (re-rasterize)   tf = CSS-transform pan/zoom (composite)');
  console.log(H);
  console.log('-'.repeat(H.length));
  for (const r of results) {
    console.log(
      `${String(r.persons).padStart(8)}${String(r.domElements).padStart(9)}${String(r.initialRenderMs).padStart(11)}` +
      `${String(r.fps_viewbox).padStart(8)}${String(r.dropped_viewbox).padStart(9)}${String(r.fps_transform).padStart(8)}${String(r.dropped_transform).padStart(9)}${String(r.selectRecalcMs).padStart(12)}`
    );
  }
  console.log('\nwrote out/results.json and out/sample-2000.png');
})().catch((e) => { console.error(e); process.exit(1); });
