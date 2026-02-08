// Allow HTMX to swap error responses (4xx/5xx) so users see error messages
document.addEventListener('htmx:beforeSwap', (evt) => {
    const status = evt.detail.xhr.status;
    if (status >= 400) {
        evt.detail.shouldSwap = true;
        evt.detail.isError = false;
    }
});

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

// Add a spec textarea to the join paste container
function addJoinSpec(container, content) {
    const slots = container.querySelectorAll('.spec-slot');
    if (slots.length >= 5) return;

    const index = slots.length + 1;
    const div = document.createElement('div');
    div.className = 'spec-slot';
    div.innerHTML = `
        <div class="spec-slot-header">
            <span class="spec-slot-label">Spec ${index}</span>
            <button type="button" class="btn-remove-spec" onclick="removeJoinSpec(this)">Remove</button>
        </div>
        <textarea name="specs_content" rows="8" placeholder="Paste OpenAPI spec (JSON or YAML)..."></textarea>
    `;
    if (content) {
        div.querySelector('textarea').value = content;
    }
    container.appendChild(div);
    updateJoinSpecState(container);
}

// Remove a spec textarea from the join paste container
function removeJoinSpec(button) {
    const container = button.closest('.specs-paste-container');
    const slots = container.querySelectorAll('.spec-slot');
    if (slots.length <= 2) return;

    button.closest('.spec-slot').remove();
    // Renumber remaining slots
    container.querySelectorAll('.spec-slot').forEach((slot, i) => {
        slot.querySelector('.spec-slot-label').textContent = `Spec ${i + 1}`;
    });
    updateJoinSpecState(container);
}

// Update add/remove button states based on current slot count
function updateJoinSpecState(container) {
    const slots = container.querySelectorAll('.spec-slot');
    const count = slots.length;

    // Disable remove when at minimum (2)
    slots.forEach(slot => {
        const btn = slot.querySelector('.btn-remove-spec');
        if (btn) btn.disabled = count <= 2;
    });

    // Disable add when at maximum (5)
    const addBtn = container.parentElement.querySelector('.btn-add-spec');
    if (addBtn) addBtn.disabled = count >= 5;
}

// Load example spec into input section
async function loadExample(select) {
    const exampleName = select.value;
    if (!exampleName) return;

    const section = select.closest('.input-section');
    if (!section) return;

    try {
        const response = await fetch(`/api/examples/${exampleName}`);
        if (!response.ok) {
            throw new Error(`Failed to load example: ${response.statusText}`);
        }
        const content = await response.text();

        // Switch to paste mode
        switchInputMode(section, 'paste');

        // Find and populate the textarea
        const textarea = section.querySelector('textarea');
        if (textarea) {
            textarea.value = content;
            textarea.disabled = false;
        }

        // Reset the select
        select.value = '';
    } catch (error) {
        console.error('Failed to load example:', error);
        alert('Failed to load example. Please try again.');
        select.value = '';
    }
}

// Load example for join page (switches to paste mode and adds a pre-filled textarea)
async function loadJoinExample(select) {
    const exampleName = select.value;
    if (!exampleName) return;

    const section = select.closest('.input-section');
    if (!section) return;

    try {
        const response = await fetch(`/api/examples/${exampleName}`);
        if (!response.ok) throw new Error(`Failed to load: ${response.statusText}`);
        const content = await response.text();

        // Switch to paste mode
        switchInputMode(section, 'paste');

        // Find the paste container
        const container = section.querySelector('.specs-paste-container');
        if (!container) {
            console.error('Failed to find paste container for example loading');
            return;
        }

        // Check if there's an empty textarea we can fill first
        const emptyTextarea = Array.from(container.querySelectorAll('textarea')).find(ta => !ta.value.trim());
        if (emptyTextarea) {
            emptyTextarea.value = content;
        } else if (container.querySelectorAll('.spec-slot').length >= 5) {
            alert('Maximum 5 specs reached. Remove a spec before adding another example.');
        } else {
            // Add a new spec slot with the content
            addJoinSpec(container, content);
        }

        select.value = '';
    } catch (error) {
        console.error('Failed to load example:', error);
        alert('Failed to load example. Please try again.');
        select.value = '';
    }
}
