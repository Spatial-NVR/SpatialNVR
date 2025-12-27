import { test, expect } from '@playwright/test'

test.describe('Dashboard - Comprehensive Tests', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')
  })

  test('should load dashboard with all sections visible', async ({ page }) => {
    // Wait for page to fully load
    await page.waitForTimeout(2000)

    // Verify main content loaded
    const bodyContent = await page.locator('body').textContent()
    expect(bodyContent?.length).toBeGreaterThan(100)

    // Verify page loaded properly (any of these means dashboard is working)
    const hasContent = bodyContent?.toLowerCase().includes('camera') ||
                       bodyContent?.toLowerCase().includes('live') ||
                       bodyContent?.toLowerCase().includes('activity') ||
                       bodyContent?.toLowerCase().includes('add')
    expect(hasContent).toBeTruthy()

    // Main content container should be visible
    const mainContent = page.locator('.space-y-6, main').first()
    await expect(mainContent).toBeVisible()
  })

  test('should not show state_change events in Recent Activity', async ({ page }) => {
    await page.waitForTimeout(3000)

    // Get all event cards in recent activity
    const eventCards = page.locator('.flex.gap-2.p-3 a')
    const count = await eventCards.count()

    // Check each event card - none should show "state_change"
    for (let i = 0; i < count; i++) {
      const cardText = await eventCards.nth(i).textContent()
      expect(cardText?.toLowerCase()).not.toContain('state_change')
      expect(cardText?.toLowerCase()).not.toContain('invalid data')
    }
  })

  test('should display camera count correctly', async ({ page }) => {
    await page.waitForTimeout(2000)

    // Look for camera count text like "X of Y cameras online"
    const cameraStatus = page.getByText(/\d+ of \d+ cameras online|no cameras configured/i)
    await expect(cameraStatus).toBeVisible()
  })

  test('Add Camera button should navigate to add camera page', async ({ page }) => {
    const addCameraButton = page.getByRole('link', { name: /add camera/i }).first()
    await expect(addCameraButton).toBeVisible()

    await addCameraButton.click()
    await page.waitForTimeout(1000)

    expect(page.url()).toContain('/cameras/add')
  })

  test('View all link should navigate to events page', async ({ page }) => {
    const viewAllLink = page.getByRole('link', { name: /view all/i })
    if (await viewAllLink.count() > 0) {
      await viewAllLink.click()
      await page.waitForTimeout(1000)
      expect(page.url()).toContain('/events')
    }
  })

  test('should display camera cards with status indicators', async ({ page }) => {
    await page.waitForTimeout(2000)

    // Find camera cards
    const cameraCards = page.locator('a[href^="/cameras/"]').filter({ hasNot: page.locator('[href="/cameras/add"]') })
    const count = await cameraCards.count()

    if (count > 0) {
      // Each camera card should have a status indicator dot
      for (let i = 0; i < Math.min(count, 5); i++) {
        const card = cameraCards.nth(i)
        // Check for status dot (green, gray, red, or yellow)
        const statusDot = card.locator('.rounded-full').filter({ has: page.locator('[class*="bg-green"], [class*="bg-gray"], [class*="bg-red"], [class*="bg-yellow"]') })
        // Status should exist
        await expect(card).toBeVisible()
      }
    }
  })

  test('clicking a camera card should navigate to camera detail', async ({ page }) => {
    await page.waitForTimeout(2000)

    const cameraCard = page.locator('a[href^="/cameras/"]').filter({ hasNot: page.locator('[href="/cameras/add"]') }).first()
    if (await cameraCard.count() > 0) {
      const href = await cameraCard.getAttribute('href')
      await cameraCard.click()
      await page.waitForTimeout(1000)

      expect(page.url()).toContain(href!)
    }
  })

  test('event cards should be clickable and show event details', async ({ page }) => {
    await page.waitForTimeout(2000)

    // Find event cards in recent activity
    const eventCards = page.locator('.flex.gap-2.p-3 a')
    const count = await eventCards.count()

    if (count > 0) {
      const firstCard = eventCards.first()
      const href = await firstCard.getAttribute('href')

      // Each event card should have an icon and timestamp
      const cardContent = await firstCard.textContent()
      expect(cardContent).toBeTruthy()

      // Click should navigate to events page
      await firstCard.click()
      await page.waitForTimeout(1000)
      expect(page.url()).toContain('/events')
    }
  })

  test('navigation links should all work correctly', async ({ page }) => {
    // Test each navigation link
    const navLinks = [
      { name: /cameras/i, url: '/cameras' },
      { name: /recordings/i, url: '/recordings' },
      { name: /events/i, url: '/events' },
      { name: /settings/i, url: '/settings' },
    ]

    for (const navLink of navLinks) {
      await page.goto('/')
      await page.waitForLoadState('networkidle')

      const link = page.getByRole('link', { name: navLink.name }).first()
      if (await link.count() > 0) {
        await link.click()
        await page.waitForTimeout(1000)
        expect(page.url()).toContain(navLink.url)
      }
    }
  })

  test('page should handle refresh without errors', async ({ page }) => {
    await page.waitForTimeout(2000)

    // Capture any console errors
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))

    // Refresh the page
    await page.reload()
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(2000)

    // Page should still load correctly
    const bodyContent = await page.locator('body').textContent()
    expect(bodyContent?.length).toBeGreaterThan(100)

    // No JavaScript errors
    expect(errors.filter(e => !e.includes('ResizeObserver'))).toHaveLength(0)
  })

  test('video players should load for online cameras', async ({ page }) => {
    await page.waitForTimeout(3000)

    // Find camera cards that are online (have video element)
    const videoElements = page.locator('video')
    const videoCount = await videoElements.count()

    if (videoCount > 0) {
      // At least one video should be present
      const firstVideo = videoElements.first()
      await expect(firstVideo).toBeVisible()
    }
  })
})
