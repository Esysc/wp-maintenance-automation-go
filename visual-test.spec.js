import { test, expect } from '@playwright/test';

const BASE = 'https://localhost:8080';
const API = 'https://localhost:8081';
const TEST_PASSWORD = process.env.WP_MAINTENANCE_TEST_PASSWORD || 'Admin123!';

test('VISUAL: full flow with DOM verification', async ({ page }) => {
  const errors = [];
  page.on('console', msg => {
    if (msg.type() === 'error') errors.push(msg.text());
  });
  page.on('pageerror', err => errors.push('PAGE ERROR: ' + err.message));

  // 1. Login page
  await page.goto(`${BASE}/login`);
  const loginTitle = await page.locator('#loginTitle').textContent();
  console.log('[1] Login title:', loginTitle);
  const hasConfirmField = await page.locator('#passwordConfirm').isVisible().catch(() => false);
  console.log('[1] Has visible passwordConfirm field:', hasConfirmField);

  // Fill and submit
  await page.fill('#password', TEST_PASSWORD);
  if (hasConfirmField) {
    await page.fill('#passwordConfirm', TEST_PASSWORD);
  }
  await page.click('#submitBtn');
  await page.waitForTimeout(3000);

  if (!page.url().includes('/dashboard')) {
    await page.click('#submitBtn');
    await page.waitForTimeout(1500);
  }

  console.log('[2] After login URL:', page.url());
  expect(page.url()).toContain('/dashboard');

  // 2. Dashboard - check content
  const dashH1 = await page.locator('h1').first().textContent().catch(() => 'none');
  console.log('[3] Dashboard H1:', dashH1);
  const sidebarLinks = await page.locator('.nav-list a').allTextContents();
  console.log('[3] Sidebar links:', sidebarLinks.join(', '));

  // 3. Sites page
  await page.goto(`${BASE}/sites`);
  await page.waitForTimeout(1500);
  const sitesH1 = await page.locator('h1').first().textContent().catch(() => 'none');
  console.log('[4] Sites H1:', sitesH1);
  const sitesListVisible = await page.locator('#sitesBody').isVisible();
  console.log('[4] Sites list visible:', sitesListVisible);
  const addSiteBtn = await page.locator('button:has-text("Add Site")').count();
  console.log('[4] Add Site button count:', addSiteBtn);

  // Click Add Site
  if (addSiteBtn > 0) {
    await page.locator('.page-header button:has-text("Add Site")').click();
    await page.waitForTimeout(500);
    const formVisible = await page.locator('#createSiteModal').isVisible();
    console.log('[5] Site form visible after click:', formVisible);

    // Fill form
    await page.fill('#siteName', 'My Test Site');
    await page.fill('#siteSSHHost', 'wp.example.com');
    await page.fill('#siteSSHUser', 'ubuntu');
    await page.fill('#siteWPRoot', '/var/www/html');

    // Submit
    await page.click('#siteCreateForm button[type="submit"]');
    await page.waitForTimeout(2000);
    
    const siteCards = await page.locator('#sitesBody .entity-card').count();
    console.log('[6] Site cards after create:', siteCards);
    const siteTitles = await page.locator('#sitesBody .entity-title').allTextContents();
    console.log('[6] Site card titles:', siteTitles.join(' | '));
  } else {
    console.log('[5] ERROR: No Add Site button found!');
  }

  // 4. Tokens page
  await page.goto(`${BASE}/tokens`);
  await page.waitForTimeout(1500);
  const tokensH1 = await page.locator('h1').first().textContent().catch(() => 'none');
  console.log('[7] Tokens H1:', tokensH1);
  const createTokenBtn = await page.locator('button:has-text("Create Token")').count();
  console.log('[7] Create Token button count:', createTokenBtn);

  if (createTokenBtn > 0) {
    await page.locator('.page-header button:has-text("Create Token")').click();
    await page.waitForTimeout(500);
    const tokenFormVisible = await page.locator('#createTokenModal').isVisible();
    console.log('[8] Token form visible:', tokenFormVisible);

    // Check form fields
    const nameInput = await page.locator('#tokenName').count();
    const durationInput = await page.locator('#tokenDuration').count();
    const userIdInput = await page.locator('#tokenUserId').count();
    console.log('[8] Form fields - name:', nameInput, 'duration:', durationInput, 'userId:', userIdInput);

    // Fill and submit
    await page.fill('#tokenName', 'Test Token');
    await page.fill('#tokenDuration', '24');
    await page.click('#tokenCreateForm button[type="submit"]');
    await page.waitForTimeout(2000);

    const tokenCards = await page.locator('#tokensBody .entity-card').count();
    console.log('[9] Token cards:', tokenCards);
    const tokenTitles = await page.locator('#tokensBody .entity-title').allTextContents();
    console.log('[9] Token card titles:', tokenTitles.join(' | '));
  } else {
    console.log('[8] ERROR: No Create Token button found!');
  }

  // 5. System
  await page.goto(`${BASE}/system`);
  await page.waitForTimeout(2000);
  const statusText = await page.locator('#statusDisplay').textContent();
  console.log('[10] Status:', statusText.substring(0, 200));
  const healthText = await page.locator('#healthDisplay').textContent();
  console.log('[11] Health:', healthText.substring(0, 200));

  // 6. API docs
  await page.goto(`${BASE}/api/docs`);
  await page.waitForTimeout(1000);
  const docsTitle = await page.title();
  console.log('[12] Docs page title:', docsTitle);
  const swaggerUI = await page.locator('.swagger-ui').count();
  console.log('[12] Swagger UI present:', swaggerUI > 0);

  // Report errors
  if (errors.length > 0) {
    console.log('\n=== BROWSER ERRORS ===');
    errors.forEach(e => console.log(' -', e));
  } else {
    console.log('\n=== NO BROWSER ERRORS ===');
  }

  expect(errors, 'Browser console/page errors found:\n' + errors.join('\n')).toEqual([]);
});
