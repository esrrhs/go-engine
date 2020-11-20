const puppeteer = require('puppeteer');
const process = require('process');

var args = process.argv.splice(2);

(async () => {
  const browser = await puppeteer.connect({browserWSEndpoint :args[1]});
  const page = await browser.newPage();
  try {
    await page.goto(args[0], {waitUntil : 'domcontentloaded'});
    console.log(await page.content());
   }
  catch(e) {
    console.log(e)
  }
  finally {
    await page.close()
    browser.disconnect()
  }
})();
