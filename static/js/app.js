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
