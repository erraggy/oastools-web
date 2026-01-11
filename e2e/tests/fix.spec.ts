import { test, expect } from '@playwright/test';
import path from 'path';

test.describe('Fix Page', () => {
  test('fixes a spec and shows results', async ({ page }) => {
    await page.goto('/fix');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/fix/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');

    // Wait for result - should show fixes or "no fixes needed"
    await expect(page.locator('#result')).toBeVisible({ timeout: 10000 });
  });

  test('can toggle advanced options', async ({ page }) => {
    await page.goto('/fix');

    // Advanced options should be collapsed by default
    const advancedContent = page.locator('.advanced-options .advanced-content');
    await expect(advancedContent).not.toBeVisible();

    // Click to expand
    await page.click('.advanced-options summary');
    await expect(advancedContent).toBeVisible();

    // Dry run checkbox should be present
    await expect(page.locator('input[name="dryRun"]')).toBeVisible();
  });

  test('new fix options are present', async ({ page }) => {
    await page.goto('/fix');

    // Main section should have duplicate operationIds option
    await expect(page.locator('input[name="fixDuplicateOperationIds"]')).toBeVisible();

    // Expand advanced options
    await page.click('.advanced-options summary');

    // New advanced options should be visible
    await expect(page.locator('input[name="expandCSVEnums"]')).toBeVisible();
  });
});
