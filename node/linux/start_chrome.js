const puppeteer = require('puppeteer');
const process = require('process');

(async () => {
  const browser = await puppeteer.launch({args: ['--no-sandbox', '--disable-setuid-sandbox'], timeout:0});
  console.log(browser.wsEndpoint());
  
})();
