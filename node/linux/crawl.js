const puppeteer = require('puppeteer');
const process = require('process');

var args = process.argv.splice(2);

(async () => {
  const browser = await puppeteer.launch({args: ['--no-sandbox', '--disable-setuid-sandbox'], timeout:0});
  try {
    const page = await browser.newPage();
    await page.goto(args[0], {waitUntil : 'networkidle0'});
    console.log(await page.content());
  } 
  catch(e) {
    console.log(e)
    process.exit()
  }
  finally {
    browser.close()
  }
})();
