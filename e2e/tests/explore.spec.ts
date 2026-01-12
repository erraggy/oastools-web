import { test, expect, Page } from '@playwright/test';
import path from 'path';

// Helper to upload spec and wait for results
async function uploadSpecAndWaitForResults(page: Page, specPath: string) {
  const fileInput = page.locator('input[name="spec"]');
  await fileInput.setInputFiles(specPath);

  // Wait for the form submission response
  const responsePromise = page.waitForResponse(resp =>
    resp.url().includes('/api/explore') && resp.request().method() === 'POST'
  );

  await page.click('button[type="submit"]');

  const response = await responsePromise;
  if (!response.ok()) {
    throw new Error(`Upload failed with status ${response.status()}: ${await response.text()}`);
  }

  // Wait for HTMX to swap content
  await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });
}

// Helper to click tab and wait for content
async function clickTabAndWait(page: Page, tabName: string, expectedSelector: string) {
  const responsePromise = page.waitForResponse(resp =>
    resp.url().includes(`/api/explore/${tabName}`)
  );

  await page.click(`button[data-tab="${tabName}"]`);

  const response = await responsePromise;
  if (!response.ok()) {
    throw new Error(`Tab request failed with status ${response.status()}: ${await response.text()}`);
  }

  await expect(page.locator(expectedSelector)).toBeVisible({ timeout: 10000 });
}

test.describe('Explore Page', () => {
  // Navigate to explore page and wait for it to be fully loaded
  test.beforeEach(async ({ page }) => {
    // Wait for full page load including all scripts
    await page.goto('/explore', { waitUntil: 'networkidle' });

    // Wait for file input to be interactive
    await expect(page.locator('input[name="spec"]')).toBeVisible({ timeout: 10000 });

    // Verify HTMX is loaded by checking for htmx attribute
    await expect(page.locator('[hx-post]')).toBeVisible({ timeout: 5000 });
  });

  test('uploads spec and displays summary stats', async ({ page }) => {
    await uploadSpecAndWaitForResults(
      page,
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );

    // Verify summary stats are displayed
    await expect(page.locator('.summary-version')).toContainText('3.0');
    await expect(page.locator('.summary-stats')).toContainText('Paths:');
    await expect(page.locator('.summary-stats')).toContainText('Operations:');
    await expect(page.locator('.summary-stats')).toContainText('Schemas:');
  });

  test('switches between tabs', async ({ page }) => {
    await uploadSpecAndWaitForResults(
      page,
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );

    // Click Operations tab
    await clickTabAndWait(page, 'operations', '.operations-panel');
    await expect(page.locator('.operations-list')).toBeVisible();

    // Click Schemas tab
    await clickTabAndWait(page, 'schemas', '.schemas-container');
    await expect(page.locator('.schemas-list')).toBeVisible();

    // Click Security tab
    await clickTabAndWait(page, 'security', '.security-container');
  });

  test('group-by dropdown changes operation grouping', async ({ page }) => {
    await uploadSpecAndWaitForResults(
      page,
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );

    // Click Operations tab first
    await clickTabAndWait(page, 'operations', '.operations-panel');

    // Default grouping is by path - verify path-like group titles
    await expect(page.locator('.group-title').first()).toContainText('/');

    // Change to group by method
    const methodResponsePromise = page.waitForResponse(resp =>
      resp.url().includes('/api/explore/operations') && resp.url().includes('group=method')
    );
    await page.selectOption('#group-by', 'method');
    await methodResponsePromise;

    // Wait for the group header to update (method view shows method badges in headers)
    await expect(page.locator('.operation-group .group-header .method-badge').first()).toBeVisible({ timeout: 10000 });

    // Change to group by tag
    const tagResponsePromise = page.waitForResponse(resp =>
      resp.url().includes('/api/explore/operations') && resp.url().includes('group=tag')
    );
    await page.selectOption('#group-by', 'tag');
    await tagResponsePromise;
  });

  test('clicks operation row to show detail', async ({ page }) => {
    await uploadSpecAndWaitForResults(
      page,
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );

    // Click Operations tab
    await clickTabAndWait(page, 'operations', '.operations-panel');

    // Click first operation row and wait for detail
    const detailResponsePromise = page.waitForResponse(resp =>
      resp.url().includes('/api/explore/operation?')
    );
    await page.locator('.operation-row').first().click();
    await detailResponsePromise;

    // Verify operation detail is displayed
    await expect(page.locator('#operation-detail .detail-card')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.detail-path')).toBeVisible();
  });

  test('clicks schema row to show detail', async ({ page }) => {
    await uploadSpecAndWaitForResults(
      page,
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );

    // Click Schemas tab
    await clickTabAndWait(page, 'schemas', '.schemas-container');

    // Click first schema row and wait for detail
    const detailResponsePromise = page.waitForResponse(resp =>
      resp.url().includes('/api/explore/schema?')
    );
    await page.locator('.schema-row').first().click();
    await detailResponsePromise;

    // Verify schema detail is displayed
    await expect(page.locator('#schema-detail .detail-card')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('#schema-detail .schema-name')).toBeVisible();
  });

  test('expands summary details', async ({ page }) => {
    await uploadSpecAndWaitForResults(
      page,
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );

    // Click the "Show details" button and wait for response
    const detailsResponsePromise = page.waitForResponse(resp =>
      resp.url().includes('/api/explore/summary-details')
    );
    await page.click('.summary-expand-btn');
    await detailsResponsePromise;

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

    // Wait for the form submission response
    const responsePromise = page.waitForResponse(resp =>
      resp.url().includes('/api/explore') && resp.request().method() === 'POST'
    );
    await page.click('button[type="submit"]');
    await responsePromise;

    // Wait for result container
    await expect(page.locator('.explore-container')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.summary-version')).toContainText('3.0');
  });

  test('works with OpenAPI 2.0 specs', async ({ page }) => {
    await uploadSpecAndWaitForResults(
      page,
      path.join(__dirname, '../../testdata/golden/explore/petstore-2.0.input.yaml')
    );

    // Verify it shows version 2.0
    await expect(page.locator('.summary-version')).toContainText('2.0');

    // Tabs should still work
    await clickTabAndWait(page, 'operations', '.operations-panel');
  });

  test('security tab shows scheme details when clicked', async ({ page }) => {
    await uploadSpecAndWaitForResults(
      page,
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );

    // Click Security tab
    await clickTabAndWait(page, 'security', '.security-container');

    // Click first security scheme row (if any exist) and wait for detail response
    const securityRow = page.locator('.security-row').first();
    if (await securityRow.isVisible()) {
      const detailResponsePromise = page.waitForResponse(resp =>
        resp.url().includes('/api/explore/security-detail')
      );
      await securityRow.click();
      await detailResponsePromise;
      await expect(page.locator('#security-detail .detail-card')).toBeVisible({ timeout: 10000 });
    }
  });
});
