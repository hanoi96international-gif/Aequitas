// Druckt build/node_guide_html/<lang>.html (aus build/node_guide.py) als PDF:
// downloads/Aequitas_Node_Guide_<LANG>.pdf, dazu die englische Fassung als
// docs/aequitas-node-guide.pdf. Braucht Playwright mit Chromium.
const path = require('path');
const { chromium } = require('playwright');

const wurzel = path.resolve(__dirname, '..');
const sprachen = ['en', 'de', 'es', 'fr', 'it', 'pt', 'tr', 'id', 'ru', 'zh', 'ar', 'hi'];

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  for (const lang of sprachen) {
    await page.goto('file://' + path.join(__dirname, 'node_guide_html', `${lang}.html`));
    await page.evaluate(() => document.fonts.ready);
    const ziel = path.join(wurzel, 'downloads', `Aequitas_Node_Guide_${lang.toUpperCase()}.pdf`);
    const opt = {
      format: 'A4', printBackground: true, preferCSSPageSize: true, displayHeaderFooter: true,
      headerTemplate: '<span></span>',
      footerTemplate: '<div style="font-size:7pt;color:#9CA3AF;width:100%;text-align:center">Aequitas · <span class="pageNumber"></span>/<span class="totalPages"></span></div>',
    };
    await page.pdf({ ...opt, path: ziel });
    if (lang === 'en') await page.pdf({ ...opt, path: path.join(wurzel, 'docs', 'aequitas-node-guide.pdf') });
    console.log(lang, ziel);
  }
  await browser.close();
})();
