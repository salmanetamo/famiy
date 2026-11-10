'use strict';
const http = require('http');
const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');

const roots = [path.join(__dirname, 'dist'), path.join(__dirname, 'public')];
const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json' };
const srv = http.createServer((q, s) => {
  const u = new URL(q.url, 'http://x');
  const f = u.pathname === '/' ? '/index.html' : u.pathname;
  for (const r of roots) {
    const p = path.join(r, f);
    if (fs.existsSync(p) && fs.statSync(p).isFile()) {
      s.writeHead(200, { 'content-type': types[path.extname(p)] || 'application/octet-stream' });
      return fs.createReadStream(p).pipe(s);
    }
  }
  s.writeHead(404);
  s.end();
});

(async () => {
  await new Promise((r) => srv.listen(0, r));
  const port = srv.address().port;
  const browser = await chromium.launch();
  const out = [];
  const cases = [['svg', 'small', 0], ['rf', 'small', 0], ['cy', 'small', 0], ['svg', '1k', 0], ['rf', '1k', 0], ['rf', '1k', 1], ['cy', '1k', 0]];
  for (const [lib, data, cull] of cases) {
    const results = [];
    for (let i = 0; i < 3; i++) {
      const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
      page.on('pageerror', (e) => console.error('pageerror', lib, e.message));
      await page.goto(`http://localhost:${port}/?lib=${lib}&data=${data}&cull=${cull}`);
      await page.waitForFunction('typeof window.runBench==="function"');
      results.push(await page.evaluate(() => window.runBench()));
      if (i === 0) await page.screenshot({ path: path.join(__dirname, `shot-${lib}-${data}${cull ? '-cull' : ''}.png`) });
      await page.close();
    }
    const med = (k) => results.map((r) => r[k]).sort((a, b) => a - b)[1];
    out.push({ lib, data, cull, persons: results[0].persons, domEls: results[0].domEls, renderMs: med('renderMs'), fps: med('medFps'), drop: med('dropPct'), selectMs: med('selectMs') });
    console.error('done', lib, data, cull);
  }
  await browser.close();
  srv.close();
  console.table(out);
  fs.writeFileSync(path.join(__dirname, 'results.json'), JSON.stringify(out, null, 2));
})().catch((e) => { console.error(e); process.exit(1); });
