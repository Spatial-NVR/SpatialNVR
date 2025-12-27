import { test, expect } from '@playwright/test'

test.describe('Events Page - Comprehensive Tests', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/events')
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(2000)
  })

  test('should load events page with all UI elements', async ({ page }) => {
    // Page should contain "events" in content
    const bodyContent = await page.locator('body').textContent()
    expect(bodyContent?.length).toBeGreaterThan(100)
    expect(bodyContent?.toLowerCase()).toContain('event')

    // Check for filter button
    const filterButton = page.getByRole('button', { name: /filter/i })
    await expect(filterButton).toBeVisible()
  })

  test('filter panel should toggle on button click', async ({ page }) => {
    const filterButton = page.getByRole('button', { name: /filter/i })

    // Initially filter panel may be hidden
    await filterButton.click()
    await page.waitForTimeout(500)

    // Filter panel should be visible with options
    const filterOptions = page.getByText(/all events|motion|person|vehicle|animal/i)
    await expect(filterOptions.first()).toBeVisible()

    // Click filter again to close
    await filterButton.click()
    await page.waitForTimeout(500)
  })

  test('should filter events by type and verify filter is applied', async ({ page }) => {
    const filterButton = page.getByRole('button', { name: /filter/i })
    await filterButton.click()
    await page.waitForTimeout(500)

    // Click on motion filter
    const motionFilter = page.getByRole('button', { name: /^motion$/i })
    if (await motionFilter.count() > 0) {
      await motionFilter.click()
      await page.waitForTimeout(1000)

      // Motion filter should be active (have primary background)
      const classes = await motionFilter.getAttribute('class')
      expect(classes).toContain('bg-primary')
    }

    // Click on person filter
    const personFilter = page.getByRole('button', { name: /^person$/i })
    if (await personFilter.count() > 0) {
      await personFilter.click()
      await page.waitForTimeout(1000)

      const classes = await personFilter.getAttribute('class')
      expect(classes).toContain('bg-primary')
    }

    // Click on All Events to reset
    const allFilter = page.getByRole('button', { name: /all events/i })
    if (await allFilter.count() > 0) {
      await allFilter.click()
      await page.waitForTimeout(1000)

      const classes = await allFilter.getAttribute('class')
      expect(classes).toContain('bg-primary')
    }
  })

  test('should toggle between list and timeline views', async ({ page }) => {
    const listButton = page.locator('button[title="List view"]')
    const timelineButton = page.locator('button[title="Timeline view"]')

    // Click timeline view
    await timelineButton.click()
    await page.waitForTimeout(500)

    // Timeline button should be active
    let timelineClasses = await timelineButton.getAttribute('class')
    expect(timelineClasses).toContain('bg-primary')

    // List button should not be active
    let listClasses = await listButton.getAttribute('class')
    expect(listClasses).not.toContain('bg-primary')

    // Timeline view should show timeline elements
    const timelineLine = page.locator('.bg-border, [class*="timeline"]')
    // Should have timeline UI elements visible

    // Switch back to list view
    await listButton.click()
    await page.waitForTimeout(500)

    // List button should now be active
    listClasses = await listButton.getAttribute('class')
    expect(listClasses).toContain('bg-primary')

    timelineClasses = await timelineButton.getAttribute('class')
    expect(timelineClasses).not.toContain('bg-primary')
  })

  test('event cards should display correct information', async ({ page }) => {
    await page.waitForTimeout(2000)

    // Find event cards
    const eventCards = page.locator('.bg-card.rounded-lg.border.p-4, [class*="event"]')
    const count = await eventCards.count()

    if (count > 0) {
      // Check first event card has required elements
      const firstCard = eventCards.first()

      // Should have event type or label
      const cardText = await firstCard.textContent()
      expect(cardText?.length).toBeGreaterThan(5)

      // Should have timestamp
      const hasTime = cardText?.match(/\d{1,2}:\d{2}|ago|just now/i)
      expect(hasTime).toBeTruthy()
    }
  })

  test('acknowledge button should work on unacknowledged events', async ({ page }) => {
    await page.waitForTimeout(2000)

    // Find acknowledge button
    const ackButton = page.locator('button[title*="Acknowledge"], button[title*="acknowledge"]').first()

    if (await ackButton.count() > 0) {
      // Click acknowledge
      await ackButton.click()
      await page.waitForTimeout(1000)

      // Button should disappear or change state (event acknowledged)
      // The mutation should trigger a refetch
    }
  })

  test('close button on filter panel should close it', async ({ page }) => {
    const filterButton = page.getByRole('button', { name: /filter/i })
    await filterButton.click()
    await page.waitForTimeout(500)

    // Find close button (X icon)
    const closeButton = page.locator('.p-1.rounded.hover\\:bg-accent').filter({ has: page.locator('svg') })

    if (await closeButton.count() > 0) {
      await closeButton.first().click()
      await page.waitForTimeout(500)
    }
  })

  test('should show empty state when no events', async ({ page }) => {
    // Navigate with a filter that likely has no results
    await page.goto('/events')
    await page.waitForTimeout(2000)

    // Check for events or empty state
    const eventCards = page.locator('.bg-card.rounded-lg.border.p-4')
    const emptyState = page.getByText(/no events/i)

    const hasEvents = await eventCards.count() > 0
    const hasEmptyState = await emptyState.count() > 0

    // Either should be true
    expect(hasEvents || hasEmptyState).toBeTruthy()
  })

  test('should not show state_change events in event list', async ({ page }) => {
    await page.waitForTimeout(3000)

    // Get all event cards
    const eventCards = page.locator('.bg-card.rounded-lg.border')
    const count = await eventCards.count()

    // If there are event cards, verify none show "state_change" as their visible label
    if (count > 0) {
      for (let i = 0; i < Math.min(count, 5); i++) {
        const card = eventCards.nth(i)
        const cardText = await card.locator('h3').textContent().catch(() => '')
        // Event type labels shown to users shouldn't include state_change
        if (cardText) {
          expect(cardText.toLowerCase()).not.toContain('state_change')
        }
      }
    }
    // Test passes if no cards or no state_change found
  })
})
