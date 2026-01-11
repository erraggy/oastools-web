import { test, expect } from '@playwright/test';
import path from 'path';

test.describe('Validate Page', () => {
  test('validates a valid OpenAPI spec', async ({ page }) => {
    await page.goto('/validate');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/validate/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');

    // Wait for result
    await expect(page.locator('#result')).toContainText('Valid', { timeout: 10000 });
  });

  test('shows errors for invalid spec', async ({ page }) => {
    await page.goto('/validate');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/validate/invalid-oas3.input.yaml'));

    await page.click('button[type="submit"]');

    // Wait for error result - page shows "Invalid" badge and "Errors" section
    await expect(page.locator('#result')).toContainText('Invalid', { timeout: 10000 });
  });

  test('can use paste input mode', async ({ page }) => {
    await page.goto('/validate');

    // Switch to paste mode
    await page.click('button[data-mode="paste"]');

    const textarea = page.locator('textarea[name="spec_content"]');
    await expect(textarea).toBeEnabled();

    await textarea.fill(`openapi: "3.0.0"
info:
  title: Test
  version: "1.0"
paths: {}`);

    await page.click('button[type="submit"]');

    await expect(page.locator('#result')).toContainText('Valid', { timeout: 10000 });
  });

  test('validateStructure option is present and checked by default', async ({ page }) => {
    await page.goto('/validate');

    // Expand advanced options
    await page.click('.advanced-options summary');

    const checkbox = page.locator('input[name="validateStructure"]');
    await expect(checkbox).toBeVisible();
    await expect(checkbox).toBeChecked();
  });
});
