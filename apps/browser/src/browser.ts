import { chromium } from 'playwright'

export const initBrowser = async () => {
  return await chromium.launch({
    headless: true,
  })
}