import { test, expect } from '@playwright/test';

const BASE = 'https://localhost:8080';
const API = 'https://localhost:8081';
const TEST_PASSWORD = process.env.WP_MAINTENANCE_TEST_PASSWORD || 'Admin123!';

function attachBrowserErrorGuard(page) {
  const errors = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      errors.push('[console] ' + msg.text());
    }
  });
  page.on('pageerror', (err) => {
    errors.push('[pageerror] ' + err.message);
  });
  return errors;
}

test.beforeEach(async ({ page }, testInfo) => {
  testInfo._browserErrors = attachBrowserErrorGuard(page);
});

test.afterEach(async ({}, testInfo) => {
  const errors = testInfo._browserErrors || [];
  expect(errors, 'Browser console/page errors found:\n' + errors.join('\n')).toEqual([]);
});

async function loginViaAPI(request) {
  const resp = await request.post(`${API}/api/v1/auth/login`, {
    data: { password: TEST_PASSWORD, passwordConfirm: TEST_PASSWORD },
    ignoreHTTPSErrors: true,
  });
  const body = await resp.json();
  return body.data?.token;
}

async function loginViaUI(page) {
  await page.goto(`${BASE}/login`);
  await page.fill('#password', TEST_PASSWORD);
  const confirmVisible = await page.locator('#passwordConfirm').isVisible().catch(() => false);
  if (confirmVisible) {
    await page.fill('#passwordConfirm', TEST_PASSWORD);
  }
  await page.click('#submitBtn');
  await page.waitForURL('**/dashboard', { timeout: 10000 });
}

test.describe('1. Login Flow', () => {
  test('first time login creates admin and redirects to dashboard', async ({ page }) => {
    await loginViaUI(page);
    expect(page.url()).toContain('/dashboard');
  });
});

test.describe('2. Sites Management', () => {
  test('create site via web form', async ({ page }) => {
    await loginViaUI(page);
    await page.goto(`${BASE}/sites`);
    await page.waitForTimeout(1000);

    // Click "Add Site" button
    await page.locator('.page-header button:has-text("Add Site")').click();
    await page.waitForTimeout(500);

    // Check form is visible
    const formVisible = await page.locator('#createSiteModal').isVisible();
    expect(formVisible).toBe(true);

    // Fill in required fields
    await page.fill('#siteName', 'Test WordPress Site');
    await page.fill('#siteSSHHost', 'example.com');
    await page.fill('#siteSSHUser', 'wpuser');
    await page.fill('#siteWPRoot', '/var/www/html');

    // Submit
    await page.click('#siteCreateForm button[type="submit"]');
    await page.waitForTimeout(2000);

    // Check card list has at least one site
    const cards = await page.locator('#sitesBody .entity-card').count();
    expect(cards).toBeGreaterThanOrEqual(1);
  });

  test('create site via API', async ({ request }) => {
    const token = await loginViaAPI(request);
    const resp = await request.post(`${API}/api/v1/sites`, {
      headers: { 'Authorization': `Bearer ${token}` },
      data: {
        name: 'API Site',
        wp_ssh_host: 'api.example.com',
        wp_ssh_user: 'wpuser',
        wp_root: '/var/www/html',
      },
      ignoreHTTPSErrors: true,
    });
    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.success).toBe(true);
    expect(body.data.name).toBe('API Site');
  });

  test('list sites via API', async ({ request }) => {
    const token = await loginViaAPI(request);
    const resp = await request.get(`${API}/api/v1/sites`, {
      headers: { 'Authorization': `Bearer ${token}` },
      ignoreHTTPSErrors: true,
    });
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body.success).toBe(true);
    expect(Array.isArray(body.data)).toBe(true);
    expect(body.data.length).toBeGreaterThanOrEqual(1);
  });

  test('delete site via API', async ({ request }) => {
    const token = await loginViaAPI(request);
    // First create a site to delete
    const createResp = await request.post(`${API}/api/v1/sites`, {
      headers: { 'Authorization': `Bearer ${token}` },
      data: { name: 'Delete Me', wp_ssh_host: 'del.example.com', wp_ssh_user: 'u', wp_root: '/' },
      ignoreHTTPSErrors: true,
    });
    const created = await createResp.json();
    const siteId = created.data.id;

    // Delete it
    const delResp = await request.delete(`${API}/api/v1/sites/${siteId}`, {
      headers: { 'Authorization': `Bearer ${token}` },
      ignoreHTTPSErrors: true,
    });
    expect(delResp.status()).toBe(200);
  });
});

test.describe('3. Token Management', () => {
  test('create token via web form', async ({ page }) => {
    await loginViaUI(page);
    await page.goto(`${BASE}/tokens`);
    await page.waitForTimeout(1000);

    // Click "Create Token" button
    await page.locator('.page-header button:has-text("Create Token")').click();
    await page.waitForTimeout(500);

    // Check form is visible
    const formVisible = await page.locator('#createTokenModal').isVisible();
    expect(formVisible).toBe(true);

    // Fill in token name
    await page.fill('#tokenName', 'E2E Test Token');
    
    // Submit
    await page.click('#tokenCreateForm button[type="submit"]');
    await page.waitForTimeout(2000);

    // Check updated card list
    const cardsAfter = await page.locator('#tokensBody .entity-card').count();
    expect(cardsAfter).toBeGreaterThanOrEqual(1);
  });

  test('create token via API', async ({ request }) => {
    const token = await loginViaAPI(request);
    const resp = await request.post(`${API}/api/v1/tokens`, {
      headers: { 'Authorization': `Bearer ${token}` },
      data: { name: 'api-test-token', duration: 24 },
      ignoreHTTPSErrors: true,
    });
    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.success).toBe(true);
    expect(body.data.token).toBeTruthy();
    expect(body.data.name).toBe('api-test-token');
  });

  test('list tokens via API', async ({ request }) => {
    const token = await loginViaAPI(request);
    const resp = await request.get(`${API}/api/v1/tokens`, {
      headers: { 'Authorization': `Bearer ${token}` },
      ignoreHTTPSErrors: true,
    });
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body.success).toBe(true);
    expect(Array.isArray(body.data)).toBe(true);
  });

  test('revoke token via API', async ({ request }) => {
    const token = await loginViaAPI(request);
    // Create a token to revoke
    const createResp = await request.post(`${API}/api/v1/tokens`, {
      headers: { 'Authorization': `Bearer ${token}` },
      data: { name: 'revoke-me', duration: 1 },
      ignoreHTTPSErrors: true,
    });
    const created = await createResp.json();
    const tokenId = created.data.id;

    // Revoke it
    const delResp = await request.delete(`${API}/api/v1/tokens/${tokenId}`, {
      headers: { 'Authorization': `Bearer ${token}` },
      ignoreHTTPSErrors: true,
    });
    expect(delResp.status()).toBe(200);
  });
});

test.describe('4. System Page', () => {
  test('system page loads with status and health', async ({ page }) => {
    await loginViaUI(page);
    await page.goto(`${BASE}/system`);
    await page.waitForTimeout(2000);

    const statusContent = await page.locator('#statusDisplay').textContent();
    expect(statusContent).toContain('ok');

    const healthContent = await page.locator('#healthDisplay').textContent();
    expect(healthContent).toContain('ok');
  });
});

test.describe('5. Dashboard', () => {
  test('dashboard loads after login', async ({ page }) => {
    await loginViaUI(page);
    expect(page.url()).toContain('/dashboard');
  });
});

test.describe('6. Password Validation', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(`${BASE}/login`);
    const confirmVisible = await page.locator('#passwordConfirm').isVisible().catch(() => false);
    test.skip(!confirmVisible, 'Setup-only validation tests require setup mode with visible confirm field');
  });

  test('rejects weak password', async ({ page }) => {
    await page.fill('#password', 'weak');
    await page.fill('#passwordConfirm', 'weak');
    await page.click('#submitBtn');
    await page.waitForTimeout(500);

    const errorEl = page.locator('#loginError');
    const errorVisible = await errorEl.isVisible();
    expect(errorVisible).toBe(true);
    const errorText = await errorEl.textContent();
    expect(errorText).toContain('at least 8 characters');
  });

  test('rejects password without uppercase', async ({ page }) => {
    await page.fill('#password', 'nouppercase1');
    await page.fill('#passwordConfirm', 'nouppercase1');
    await page.click('#submitBtn');
    await page.waitForTimeout(500);

    const errorEl = page.locator('#loginError');
    const errorVisible = await errorEl.isVisible();
    expect(errorVisible).toBe(true);
  });

  test('rejects mismatched passwords', async ({ page }) => {
    await page.fill('#password', 'Admin123!');
    await page.fill('#passwordConfirm', 'Different1!');
    await page.click('#submitBtn');
    await page.waitForTimeout(500);

    const errorEl = page.locator('#loginError');
    const errorVisible = await errorEl.isVisible();
    expect(errorVisible).toBe(true);
    const errorText = await errorEl.textContent();
    expect(errorText).toContain('do not match');
  });
});

test.describe('7. Unauthorized Access', () => {
  test('redirects to login when no token', async ({ page }) => {
    await page.goto(`${BASE}/dashboard`);
    await page.waitForTimeout(1000);
    expect(page.url()).toContain('/login');
  });

  test('API returns 401 without token', async ({ request }) => {
    const resp = await request.get(`${API}/api/v1/sites`, {
      ignoreHTTPSErrors: true,
    });
    expect(resp.status()).toBe(401);
  });
});
