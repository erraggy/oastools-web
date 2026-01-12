import { test, expect } from '@playwright/test';
import path from 'path';

// These tests can be flaky due to HTMX timing, so we add retries
test.describe.configure({ retries: 2 });

test.describe('Explore Page', () => {
  // Navigate to explore page and wait for it to be fully loaded
  test.beforeEach(async ({ page }) => {
    await page.goto('/explore', { waitUntil: 'networkidle' });
    await expect(page.locator('input[name="spec"]')).toBeVisible({ timeout: 10000 });
  });

  test('uploads spec and displays summary stats', async ({ page }) => {
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');

    // Wait for result container
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });

    // Verify summary stats are displayed
    await expect(page.locator('.summary-version')).toContainText('3.0');
    await expect(page.locator('.summary-stats')).toContainText('Paths:');
    await expect(page.locator('.summary-stats')).toContainText('Operations:');
    await expect(page.locator('.summary-stats')).toContainText('Schemas:');
  });

  test('switches between tabs', async ({ page }) => {
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });

    // Click Operations tab - wait for element to appear after HTMX swap
    await page.click('button[data-tab="operations"]');
    await expect(page.locator('.operations-panel')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.operations-list')).toBeVisible();

    // Click Schemas tab - wait for element to appear after HTMX swap
    await page.click('button[data-tab="schemas"]');
    await expect(page.locator('.schemas-container')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.schemas-list')).toBeVisible();

    // Click Security tab - wait for element to appear after HTMX swap
    await page.click('button[data-tab="security"]');
    await expect(page.locator('.security-container')).toBeVisible({ timeout: 10000 });
  });

  test('group-by dropdown changes operation grouping', async ({ page }) => {
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });

    // Click Operations tab first - wait for panel
    await page.click('button[data-tab="operations"]');
    await expect(page.locator('.operations-panel')).toBeVisible({ timeout: 10000 });

    // Default grouping is by path - verify path-like group titles
    await expect(page.locator('.group-title').first()).toContainText('/');

    // Change to group by method - wait for refresh
    await page.selectOption('#group-by', 'method');
    // Wait for the group header to update (method view shows method badges in headers)
    await expect(page.locator('.operation-group .group-header .method-badge').first()).toBeVisible({ timeout: 10000 });

    // Change to group by tag
    await page.selectOption('#group-by', 'tag');
    await expect(page.locator('#tab-content')).toBeVisible({ timeout: 10000 });
  });

  test('clicks operation row to show detail', async ({ page }) => {
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });

    // Click Operations tab - wait for panel
    await page.click('button[data-tab="operations"]');
    await expect(page.locator('.operations-panel')).toBeVisible({ timeout: 10000 });

    // Click first operation row
    await page.locator('.operation-row').first().click();

    // Verify operation detail is displayed
    await expect(page.locator('#operation-detail .detail-card')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.detail-path')).toBeVisible();
  });

  test('clicks schema row to show detail', async ({ page }) => {
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });

    // Click Schemas tab - wait for container
    await page.click('button[data-tab="schemas"]');
    await expect(page.locator('.schemas-container')).toBeVisible({ timeout: 10000 });

    // Click first schema row
    await page.locator('.schema-row').first().click();

    // Verify schema detail is displayed
    await expect(page.locator('#schema-detail .detail-card')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('#schema-detail .schema-name')).toBeVisible();
  });

  test('expands summary details', async ({ page }) => {
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });

    // Click the "Show details" button
    await page.click('.summary-expand-btn');

    // Verify summary details are displayed
    await expect(page.locator('.summary-details')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.method-breakdown')).toBeVisible();
    // Use first() to avoid strict mode violation (there are 2 breakdown-list elements)
    await expect(page.locator('.breakdown-list').first()).toBeVisible();
  });

  test('can use paste input mode', async ({ page }) => {
    // Switch to paste mode
    await page.click('button[data-mode="paste"]');

    const textarea = page.locator('textarea[name="spec_content"]');
    await expect(textarea).toBeEnabled();

    await textarea.fill(`openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
paths:
  /test:
    get:
      operationId: getTest
      responses:
        "200":
          description: Success`);

    await page.click('button[type="submit"]');

    // Wait for result container
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.summary-version')).toContainText('3.0');
  });

  test('works with OpenAPI 2.0 specs', async ({ page }) => {
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/explore/petstore-2.0.input.yaml'));

    await page.click('button[type="submit"]');

    // Wait for result container
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });

    // Verify it shows version 2.0
    await expect(page.locator('.summary-version')).toContainText('2.0');

    // Tabs should still work
    await page.click('button[data-tab="operations"]');
    await expect(page.locator('.operations-panel')).toBeVisible({ timeout: 10000 });
  });

  test('security tab shows scheme details when clicked', async ({ page }) => {
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });

    // Click Security tab - wait for container
    await page.click('button[data-tab="security"]');
    await expect(page.locator('.security-container')).toBeVisible({ timeout: 10000 });

    // Click first security scheme row (if any exist)
    const securityRow = page.locator('.security-row').first();
    if (await securityRow.isVisible()) {
      await securityRow.click();
      await expect(page.locator('#security-detail .detail-card')).toBeVisible({ timeout: 10000 });
    }
  });
});
