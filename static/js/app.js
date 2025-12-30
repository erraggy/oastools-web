// Re-highlight code blocks after HTMX swaps in new content
document.addEventListener('htmx:afterSwap', (evt) => {
    if (typeof hljs !== 'undefined') {
        evt.detail.target.querySelectorAll('pre code[class*="language-"]').forEach((block) => {
            const lang = block.className.match(/language-(\w+)/);
            if (lang && lang[1]) {
                try {
                    const result = hljs.highlight(block.textContent, {language: lang[1]});
                    block.innerHTML = result.value;
                    block.classList.add('hljs');
                } catch {
                    // Language not registered, skip highlighting
                }
            }
        });
    }
});

// Switch input mode (file, paste, url) within a section
function switchInputMode(section, mode) {
    // Update tabs
    section.querySelectorAll('.input-tab').forEach(tab => {
        tab.classList.toggle('active', tab.dataset.mode === mode);
    });

    // Update content visibility and disable hidden inputs
    section.querySelectorAll('.input-content').forEach(content => {
        const isActive = content.dataset.mode === mode;
        content.classList.toggle('hidden', !isActive);

        // Disable/enable inputs based on visibility
        content.querySelectorAll('input, textarea').forEach(input => {
            input.disabled = !isActive;
            if (!isActive) {
                input.removeAttribute('required');
            } else if (input.dataset.required === 'true') {
                input.setAttribute('required', '');
            }
        });
    });

    // Update hidden mode field
    const modeInput = section.querySelector('input[name$="_mode"]') ||
                      section.querySelector('input[name="input_mode"]');
    if (modeInput) {
        modeInput.value = mode;
    }
}

// Copy text to clipboard
function copyToClipboard(text, button) {
    navigator.clipboard.writeText(text).then(() => {
        const originalText = button.textContent;
        button.textContent = 'Copied!';
        setTimeout(() => {
            button.textContent = originalText;
        }, 2000);
    }).catch(err => {
        console.error('Failed to copy:', err);
    });
}

// Download text as file
function downloadAsFile(content, filename, mimeType) {
    const blob = new Blob([content], { type: mimeType });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// Add file input for join operation
function addFileInput(container) {
    const inputs = container.querySelectorAll('input[type="file"]');
    if (inputs.length >= 5) {
        return; // Max 5 files
    }

    const div = document.createElement('div');
    div.className = 'form-group';
    div.innerHTML = `
        <input type="file" name="spec[]" accept=".json,.yaml,.yml" required>
        <button type="button" class="btn-remove" onclick="removeFileInput(this)">Remove</button>
    `;
    container.appendChild(div);
}

// Remove file input
function removeFileInput(button) {
    const container = button.closest('.file-inputs');
    const inputs = container.querySelectorAll('input[type="file"]');
    if (inputs.length > 2) {
        button.parentElement.remove();
    }
}
