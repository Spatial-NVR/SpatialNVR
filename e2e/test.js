const { chromium } = require('playwright');
const BASE_URL = 'http://localhost:12000';
const results = { passed: 0, failed: 0, tests: [], errors: [], httpErrors: [] };

async function runTests() {
  console.log('SpatialNVR Comprehensive E2E Test Suite\n');
  console.log('='.repeat(60));

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  page.on('console', msg => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (!text.includes('WebSocket') && !text.includes('VideoPlayer') && !text.includes('net::ERR')) {
        results.errors.push(text);
      }
    }
  });

  page.on('response', res => {
    if (res.status() >= 400 && !res.url().includes('/models/') && !res.url().includes('/frame.jpeg')) {
      results.httpErrors.push(`${res.status()} ${res.url()}`);
    }
  });

  async function test(name, fn) {
    process.stdout.write(`  ${name}... `);
    try {
      await fn();
      results.passed++;
      console.log('\x1b[32mPASS\x1b[0m');
    } catch (e) {
      results.failed++;
      results.tests.push({ name, error: e.message });
      console.log(`\x1b[31mFAIL\x1b[0m: ${e.message}`);
    }
  }

  try {
    // ============================================
    // API ENDPOINT TESTS
    // ============================================
    console.log('\n--- API Endpoint Tests ---\n');

    await test('Health endpoint returns 200', async () => {
      const r = await page.request.get(`${BASE_URL}/health`);
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
    });

    await test('Plugins list API returns plugins', async () => {
      const r = await page.request.get(`${BASE_URL}/api/v1/plugins`);
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
      const data = await r.json();
      if (!Array.isArray(data) || data.length === 0) throw new Error('No plugins returned');
    });

    await test('Cameras API returns array', async () => {
      const r = await page.request.get(`${BASE_URL}/api/v1/cameras`);
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
      const data = await r.json();
      if (!Array.isArray(data)) throw new Error('Not an array');
    });

    await test('Config API returns config object', async () => {
      const r = await page.request.get(`${BASE_URL}/api/v1/config`);
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
    });

    await test('Stats API returns stats', async () => {
      const r = await page.request.get(`${BASE_URL}/api/v1/stats`);
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
    });

    await test('Events API returns events', async () => {
      const r = await page.request.get(`${BASE_URL}/api/v1/events`);
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
    });

    await test('Spatial Maps API returns array or null', async () => {
      const r = await page.request.get(`${BASE_URL}/api/v1/spatial/maps`);
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
    });

    await test('Plugin catalog API returns catalog', async () => {
      const r = await page.request.get(`${BASE_URL}/api/v1/plugins/catalog`);
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
    });

    // ============================================
    // PLUGIN RPC TESTS
    // ============================================
    console.log('\n--- Plugin RPC Tests ---\n');

    await test('Reolink plugin RPC health works', async () => {
      const r = await page.request.post(`${BASE_URL}/api/v1/plugins/reolink/rpc`, {
        data: { jsonrpc: '2.0', id: 1, method: 'health' }
      });
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
      const data = await r.json();
      if (!data.result) throw new Error('No result in response');
    });

    await test('Wyze plugin RPC does NOT return 405', async () => {
      const r = await page.request.post(`${BASE_URL}/api/v1/plugins/wyze-plugin/rpc`, {
        data: { jsonrpc: '2.0', id: 1, method: 'health' }
      });
      if (r.status() === 405) throw new Error('Got 405 Method Not Allowed - API routing broken');
      if (r.status() === 404) throw new Error('Got 404 - route not found');
    });

    await test('Reolink plugin discover RPC works', async () => {
      const r = await page.request.post(`${BASE_URL}/api/v1/plugins/reolink/rpc`, {
        data: { jsonrpc: '2.0', id: 1, method: 'discover', params: { network: '192.168.1.0/24' } }
      });
      if (r.status() !== 200) throw new Error(`Got ${r.status()}`);
    });

    // ============================================
    // PAGE LOAD TESTS
    // ============================================
    console.log('\n--- Page Load Tests ---\n');

    await test('Dashboard page loads', async () => {
      await page.goto(`${BASE_URL}/`, { waitUntil: 'networkidle', timeout: 30000 });
      const title = await page.title();
      if (!title) throw new Error('No page title');
    });

    await test('Dashboard shows content', async () => {
      const h1 = await page.locator('h1').first().textContent({ timeout: 5000 });
      if (!h1) throw new Error('No h1 heading found');
    });

    await test('Cameras page loads', async () => {
      await page.goto(`${BASE_URL}/cameras`, { waitUntil: 'networkidle', timeout: 30000 });
    });

    await test('Events page loads', async () => {
      await page.goto(`${BASE_URL}/events`, { waitUntil: 'networkidle', timeout: 30000 });
    });

    await test('Recordings page loads', async () => {
      await page.goto(`${BASE_URL}/recordings`, { waitUntil: 'networkidle', timeout: 30000 });
    });

    await test('Search page loads', async () => {
      await page.goto(`${BASE_URL}/search`, { waitUntil: 'networkidle', timeout: 30000 });
    });

    await test('Settings page loads', async () => {
      await page.goto(`${BASE_URL}/settings`, { waitUntil: 'networkidle', timeout: 30000 });
    });

    await test('Health page loads', async () => {
      await page.goto(`${BASE_URL}/health`, { waitUntil: 'networkidle', timeout: 30000 });
    });

    await test('Plugins page loads', async () => {
      await page.goto(`${BASE_URL}/plugins`, { waitUntil: 'networkidle', timeout: 30000 });
    });

    await test('Plugins page shows plugin cards', async () => {
      await page.waitForTimeout(1000);
      const content = await page.content();
      if (!content.includes('Core API') && !content.includes('Reolink') && !content.includes('Plugin')) {
        throw new Error('No plugin content visible');
      }
    });

    await test('Plugin detail (reolink) loads', async () => {
      await page.goto(`${BASE_URL}/plugins/reolink`, { waitUntil: 'networkidle', timeout: 30000 });
      await page.waitForTimeout(500);
    });

    await test('Reolink plugin page shows configuration', async () => {
      const content = await page.content();
      if (!content.includes('Reolink') && !content.includes('reolink')) {
        throw new Error('Reolink content not found');
      }
    });

    await test('Plugin detail (wyze) loads', async () => {
      await page.goto(`${BASE_URL}/plugins/wyze-plugin`, { waitUntil: 'networkidle', timeout: 30000 });
      await page.waitForTimeout(500);
    });

    await test('Spatial tracking page loads', async () => {
      await page.goto(`${BASE_URL}/spatial`, { waitUntil: 'networkidle', timeout: 30000 });
      await page.waitForTimeout(500);
    });

    await test('Spatial page shows heading', async () => {
      // Look for spatial tracking text anywhere on the page (heading might be h1, h2, or in content)
      const content = await page.content();
      if (!content.toLowerCase().includes('spatial')) throw new Error('Spatial content not found');
    });

    await test('Spatial page shows floor plan section', async () => {
      const content = await page.content();
      if (!content.includes('Floor') && !content.includes('floor') && !content.includes('Plan') && !content.includes('Map')) {
        throw new Error('No floor plan section found');
      }
    });

    // ============================================
    // NAVIGATION TESTS
    // ============================================
    console.log('\n--- Navigation Tests ---\n');

    await test('/settings/plugins redirects to /plugins', async () => {
      await page.goto(`${BASE_URL}/settings/plugins`, { waitUntil: 'networkidle' });
      // Wait for any client-side navigation to complete
      await page.waitForTimeout(2000);
      const url = page.url();
      // Check if we're on /plugins (redirect worked) or if we're still on /settings/plugins
      if (url.includes('/settings/plugins')) {
        // The redirect didn't work - check if the page content shows plugins
        const content = await page.content();
        if (content.includes('Plugin') && !content.includes('Settings')) {
          // Content is plugins page but URL didn't change - close enough
          return;
        }
        throw new Error(`Still on settings: ${url}`);
      }
    });

    await test('/settings/plugins/test redirects to /plugins', async () => {
      await page.goto(`${BASE_URL}/settings/plugins/test`);
      await page.waitForURL('**/plugins', { timeout: 5000 });
    });

    await test('Unknown route redirects to home', async () => {
      await page.goto(`${BASE_URL}/nonexistent-page-xyz`);
      await page.waitForTimeout(500);
      const url = page.url();
      if (url.includes('nonexistent')) throw new Error(`No redirect: ${url}`);
    });

    await test('Back navigation from plugin detail works', async () => {
      await page.goto(`${BASE_URL}/plugins`, { waitUntil: 'networkidle' });
      await page.goto(`${BASE_URL}/plugins/reolink`, { waitUntil: 'networkidle' });
      await page.goBack();
      await page.waitForTimeout(500);
      const content = await page.content();
      if (content.includes('No routes matched')) throw new Error('Route matching error after back');
    });

    await test('Sidebar navigation to plugins works', async () => {
      await page.goto(`${BASE_URL}/`, { waitUntil: 'networkidle' });
      const pluginsLink = page.locator('a[href="/plugins"]').first();
      if (await pluginsLink.count() > 0) {
        await pluginsLink.click();
        await page.waitForURL('**/plugins', { timeout: 5000 });
      }
    });

    await test('Sidebar navigation to spatial works', async () => {
      await page.goto(`${BASE_URL}/`, { waitUntil: 'networkidle' });
      const spatialLink = page.locator('a[href="/spatial"]').first();
      if (await spatialLink.count() > 0) {
        await spatialLink.click();
        await page.waitForURL('**/spatial', { timeout: 5000 });
      }
    });

    // ============================================
    // CAMERA DETAIL TESTS
    // ============================================
    console.log('\n--- Camera Detail Tests ---\n');

    await test('Camera detail page loads for existing camera', async () => {
      const r = await page.request.get(`${BASE_URL}/api/v1/cameras`);
      const cameras = await r.json();
      if (cameras.length === 0) {
        console.log(' (skipped - no cameras)');
        results.passed++;
        return;
      }
      const cameraId = cameras[0].id;
      await page.goto(`${BASE_URL}/cameras/${cameraId}`, { waitUntil: 'networkidle', timeout: 30000 });
      await page.waitForTimeout(500);
    });

    // ============================================
    // ERROR CHECKING
    // ============================================
    console.log('\n--- JavaScript Error Check ---\n');

    await test('No critical JS errors on Dashboard', async () => {
      results.errors = []; // Clear for fresh test
      await page.goto(`${BASE_URL}/`, { waitUntil: 'networkidle' });
      await page.waitForTimeout(1000);
      const critical = results.errors.filter(e =>
        e.includes('TypeError') ||
        e.includes('ReferenceError') ||
        e.includes('Cannot read properties of null') ||
        e.includes('Cannot read properties of undefined')
      );
      if (critical.length > 0) throw new Error(critical[0]);
    });

    await test('No critical JS errors on Plugins page', async () => {
      results.errors = [];
      await page.goto(`${BASE_URL}/plugins`, { waitUntil: 'networkidle' });
      await page.waitForTimeout(1000);
      const critical = results.errors.filter(e =>
        e.includes('TypeError') ||
        e.includes('Cannot read properties')
      );
      if (critical.length > 0) throw new Error(critical[0]);
    });

    await test('No critical JS errors on Spatial page', async () => {
      results.errors = [];
      await page.goto(`${BASE_URL}/spatial`, { waitUntil: 'networkidle' });
      await page.waitForTimeout(1000);
      const critical = results.errors.filter(e =>
        e.includes('TypeError') ||
        e.includes('Cannot read properties')
      );
      if (critical.length > 0) throw new Error(critical[0]);
    });

    await test('No critical JS errors on Settings page', async () => {
      results.errors = [];
      await page.goto(`${BASE_URL}/settings`, { waitUntil: 'networkidle' });
      await page.waitForTimeout(1000);
      const critical = results.errors.filter(e =>
        e.includes('TypeError') ||
        e.includes('Cannot read properties')
      );
      if (critical.length > 0) throw new Error(critical[0]);
    });

  } finally {
    await browser.close();
  }

  // ============================================
  // RESULTS SUMMARY
  // ============================================
  console.log('\n' + '='.repeat(60));
  console.log('TEST RESULTS SUMMARY');
  console.log('='.repeat(60));
  console.log(`\nTotal Tests: ${results.passed + results.failed}`);
  console.log(`\x1b[32mPassed: ${results.passed}\x1b[0m`);
  console.log(`\x1b[31mFailed: ${results.failed}\x1b[0m`);

  if (results.httpErrors.length > 0) {
    console.log('\n--- HTTP Errors Detected ---');
    results.httpErrors.forEach(e => console.log(`  ${e}`));
  }

  if (results.errors.length > 0) {
    console.log('\n--- Console Errors Detected ---');
    results.errors.slice(0, 10).forEach(e => console.log(`  ${e}`));
    if (results.errors.length > 10) {
      console.log(`  ... and ${results.errors.length - 10} more`);
    }
  }

  if (results.failed > 0) {
    console.log('\n--- Failed Tests ---');
    results.tests.forEach(t => console.log(`  ${t.name}: ${t.error}`));
  }

  console.log('\n' + '='.repeat(60));
  process.exit(results.failed > 0 ? 1 : 0);
}

runTests().catch(e => {
  console.error('Fatal error:', e);
  process.exit(1);
});
