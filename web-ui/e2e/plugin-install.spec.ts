import { test, expect } from '@playwright/test'

/**
 * Plugin Installation E2E Test
 *
 * Tests the complete flow of installing the Wyze plugin from the UI.
 * This test reproduces the issue where the plugin is installed but
 * fails to register/hot-reload properly.
 */
test.describe('Plugin Installation Flow', () => {
  test.beforeEach(async ({ page }) => {
    // Enable console logging to see API responses
    page.on('console', msg => {
      if (msg.type() === 'error' || msg.text().includes('plugin')) {
        console.log(`[Browser ${msg.type()}] ${msg.text()}`)
      }
    })

    // Monitor network requests for plugin API calls
    page.on('response', async response => {
      const url = response.url()
      if (url.includes('/api/v1/plugins')) {
        console.log(`[API] ${response.request().method()} ${url} -> ${response.status()}`)
        try {
          const body = await response.json()
          console.log(`[API Response] ${JSON.stringify(body, null, 2)}`)
        } catch {
          // Response might not be JSON
        }
      }
    })
  })

  test('should install Wyze plugin from catalog', async ({ page }) => {
    // Navigate to plugins page
    await page.goto('/plugins')
    await page.waitForLoadState('networkidle')

    // Wait for catalog to load
    await page.waitForTimeout(3000)

    // Take screenshot of initial state
    await page.screenshot({ path: 'test-results/plugin-install-01-initial.png', fullPage: true })

    // Check if Wyze is already installed
    const installedSection = page.locator('section').filter({ hasText: /installed/i })
    const installedWyze = installedSection.locator('.bg-card').filter({ hasText: /wyze/i }).first()

    if (await installedWyze.count() > 0) {
      console.log('Wyze plugin is already installed - uninstalling first')
      // Find and click the uninstall button (trash icon)
      const uninstallBtn = installedWyze.locator('button[title="Uninstall"]')
      console.log('Uninstall button count:', await uninstallBtn.count())

      if (await uninstallBtn.count() > 0) {
        // Take screenshot before clicking uninstall
        await page.screenshot({ path: 'test-results/plugin-install-01a-before-uninstall.png', fullPage: true })

        await uninstallBtn.click()
        console.log('Clicked uninstall button')

        // Wait for the success toast to appear (toast container is in .fixed.bottom-4.right-4)
        const toastSelector = '.fixed.bottom-4.right-4 > div'
        await page.waitForSelector(toastSelector, { timeout: 15000 })
        console.log('Uninstall toast appeared')

        // Take screenshot after uninstall
        await page.screenshot({ path: 'test-results/plugin-install-01b-after-uninstall.png', fullPage: true })

        // Wait for the API to complete
        await page.waitForTimeout(2000)

        // Reload the page to get fresh state
        await page.reload()
        await page.waitForLoadState('networkidle')

        // Wait for page to fully render
        await page.waitForTimeout(3000)

        // Take screenshot after reload
        await page.screenshot({ path: 'test-results/plugin-install-01c-after-reload.png', fullPage: true })
      }
    }

    // Take screenshot before install
    await page.screenshot({ path: 'test-results/plugin-install-02-before-install.png', fullPage: true })

    // Now find the Wyze plugin in Available section
    const availableSection = page.locator('section').filter({ hasText: /available/i })
    const wyzePluginCard = availableSection.locator('.bg-card').filter({ hasText: /wyze/i }).first()

    console.log('Available section found:', await availableSection.count() > 0)
    console.log('Wyze card in available:', await wyzePluginCard.count())

    // Find the Install button within the Wyze card
    const installButton = wyzePluginCard.locator('button').filter({ hasText: /install/i })

    expect(await installButton.count()).toBeGreaterThan(0)
    console.log('Found Install button, clicking...')

    // Click install
    await installButton.click()

    // Wait for installation to complete
    console.log('Waiting for installation...')
    await page.waitForTimeout(10000) // Give it 10 seconds for clone + install

    // Take screenshot after install
    await page.screenshot({ path: 'test-results/plugin-install-03-after-install.png', fullPage: true })

    // Check for success toast
    const toast = page.locator('[class*="toast"], [role="status"]').filter({ hasText: /success|installed/i })
    const toastVisible = await toast.count() > 0
    console.log(`Success toast visible: ${toastVisible}`)

    // Check for error toast
    const errorToast = page.locator('[class*="toast"], [role="status"]').filter({ hasText: /error|failed/i })
    const errorVisible = await errorToast.count() > 0
    if (errorVisible) {
      const errorText = await errorToast.first().textContent()
      console.log(`ERROR TOAST: ${errorText}`)
    }

    // Refresh the catalog
    await page.waitForTimeout(2000)
    const refreshButton = page.locator('button').filter({ has: page.locator('svg') }).filter({ hasText: '' }).first()

    // Reload the page to get fresh state
    await page.reload()
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(3000)

    // Take screenshot of final state
    await page.screenshot({ path: 'test-results/plugin-install-04-final.png', fullPage: true })

    // Verify Wyze plugin is now in installed section
    const finalInstalledSection = page.locator('section').filter({ hasText: /installed/i })
    const finalInstalledWyze = finalInstalledSection.locator('.bg-card').filter({ hasText: /wyze/i })

    const wyzeInstalled = await finalInstalledWyze.count() > 0
    console.log(`Wyze plugin in installed section: ${wyzeInstalled}`)

    if (wyzeInstalled) {
      // Check the plugin status
      const statusDot = finalInstalledWyze.locator('.rounded-full').first()
      const statusClass = await statusDot.getAttribute('class')
      console.log(`Plugin status indicator class: ${statusClass}`)

      // Get version shown
      const versionText = await finalInstalledWyze.textContent()
      console.log(`Plugin card content: ${versionText}`)
    }

    expect(wyzeInstalled).toBe(true)
  })

  test('should install plugin from GitHub URL input', async ({ page }) => {
    await page.goto('/plugins')
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(2000)

    // Find the GitHub install input
    const githubInput = page.locator('input[placeholder*="github"]')
    expect(await githubInput.count()).toBeGreaterThan(0)

    // Type the Wyze plugin repo URL
    await githubInput.fill('github.com/Spatial-NVR/wyze-plugin')

    // Take screenshot
    await page.screenshot({ path: 'test-results/plugin-github-install-01.png', fullPage: true })

    // Find the install button next to the input
    const installButton = page.locator('button').filter({ hasText: /install/i }).last()

    // Click install
    await installButton.click()

    // Wait for installation
    console.log('Waiting for GitHub installation...')
    await page.waitForTimeout(15000)

    // Take screenshot
    await page.screenshot({ path: 'test-results/plugin-github-install-02.png', fullPage: true })

    // Reload and check
    await page.reload()
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(2000)

    // Take final screenshot
    await page.screenshot({ path: 'test-results/plugin-github-install-03.png', fullPage: true })
  })

  test('should show plugin health status after install', async ({ page }) => {
    await page.goto('/plugins')
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(2000)

    // Find Wyze plugin (either installed or in catalog)
    const wyzeCard = page.locator('.bg-card').filter({ hasText: /wyze/i }).first()

    if (await wyzeCard.count() > 0) {
      const cardText = await wyzeCard.textContent()
      console.log(`Wyze card content: ${cardText}`)

      // Check for version
      const hasVersion = cardText?.includes('v2.') || cardText?.includes('2.0')
      console.log(`Has version: ${hasVersion}`)

      // Check for status indicator
      const statusDot = wyzeCard.locator('.rounded-full')
      if (await statusDot.count() > 0) {
        const statusClass = await statusDot.first().getAttribute('class')
        console.log(`Status class: ${statusClass}`)

        // Green = running, Gray = stopped, Red = error
        if (statusClass?.includes('green')) {
          console.log('Plugin is RUNNING')
        } else if (statusClass?.includes('red')) {
          console.log('Plugin has ERROR')
        } else if (statusClass?.includes('gray')) {
          console.log('Plugin is STOPPED')
        }
      }
    }

    await page.screenshot({ path: 'test-results/plugin-health-status.png', fullPage: true })
  })

  test('verify plugin API responses', async ({ page }) => {
    // Direct API test to see what the backend returns
    await page.goto('/plugins')
    await page.waitForLoadState('networkidle')

    // Make API call directly
    const catalogResponse = await page.evaluate(async () => {
      const response = await fetch('/api/v1/plugins/catalog')
      return response.json()
    })

    console.log('Catalog response:', JSON.stringify(catalogResponse, null, 2))

    // Check if wyze is in the catalog
    const wyzePlugin = catalogResponse.plugins?.find((p: { id: string }) => p.id === 'wyze')
    if (wyzePlugin) {
      console.log('Wyze plugin in catalog:', JSON.stringify(wyzePlugin, null, 2))
    } else {
      console.log('Wyze plugin NOT found in catalog')
    }

    // Check installed plugins
    const pluginsResponse = await page.evaluate(async () => {
      const response = await fetch('/api/v1/plugins')
      return response.json()
    })

    console.log('Installed plugins:', JSON.stringify(pluginsResponse, null, 2))
  })

  test('check system health version', async ({ page }) => {
    await page.goto('/health')
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(2000)

    // Take screenshot
    await page.screenshot({ path: 'test-results/health-page.png', fullPage: true })

    // Get health API response directly
    const healthResponse = await page.evaluate(async () => {
      const response = await fetch('/health')
      return response.json()
    })

    console.log('Health response:', JSON.stringify(healthResponse, null, 2))
    console.log('Version:', healthResponse.version)

    // Check the version shown on the page
    const versionText = await page.locator('body').textContent()
    const versionMatch = versionText?.match(/v?\d+\.\d+\.\d+/g)
    console.log('Versions found on page:', versionMatch)
  })
})
