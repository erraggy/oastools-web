// Explore page JavaScript
// Handles sessionStorage caching, 410 recovery, and UI interactions

(function() {
    'use strict';

    const STORAGE_KEY = 'oastools_explore_spec';

    // Store file content when user selects a file (before form submission)
    // This ensures content is available for 410 recovery even if server restarts
    document.addEventListener('change', function(evt) {
        const target = evt.target;

        if (target.name === 'spec' && target.files && target.files[0]) {
            // File input changed - read and store the content
            const file = target.files[0];
            const reader = new FileReader();
            reader.onload = function(e) {
                sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
                    content: e.target.result,
                    filename: file.name,
                    timestamp: Date.now()
                }));
            };
            reader.readAsText(file);
        } else if (target.name === 'spec_content' || target.name === 'spec_url') {
            // Clear stored spec when other inputs change
            // (actual content will be stored on form submit)
            sessionStorage.removeItem(STORAGE_KEY);
        }
    });

    // Store paste/URL content on form submission (these are synchronous)
    document.addEventListener('htmx:configRequest', function(evt) {
        // Only intercept POST to /api/explore
        if (evt.detail.path !== '/api/explore' || evt.detail.verb !== 'post') {
            return;
        }

        const form = evt.detail.elt.closest('form');
        if (!form) {
            return;
        }

        const inputMode = form.querySelector('input[name="input_mode"]');
        const mode = inputMode ? inputMode.value : 'file';

        // File content is already stored on input change, just handle paste/URL
        if (mode === 'paste') {
            const textarea = form.querySelector('textarea[name="spec_content"]');
            if (textarea && textarea.value) {
                sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
                    content: textarea.value,
                    filename: 'pasted-spec.yaml',
                    timestamp: Date.now()
                }));
            }
        } else if (mode === 'url') {
            // For URL mode, we store the URL - the server fetches it
            const urlInput = form.querySelector('input[name="spec_url"]');
            if (urlInput && urlInput.value) {
                sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
                    url: urlInput.value,
                    timestamp: Date.now()
                }));
            }
        }
    });

    // Handle 410 Gone responses (cache miss)
    document.addEventListener('htmx:responseError', function(evt) {
        if (evt.detail.xhr.status !== 410) {
            return;
        }

        const storedData = sessionStorage.getItem(STORAGE_KEY);
        if (!storedData) {
            showCacheMissMessage(evt.detail.target);
            return;
        }

        let parsed;
        try {
            parsed = JSON.parse(storedData);
        } catch {
            showCacheMissMessage(evt.detail.target);
            return;
        }

        // Check if stored data is too old (30 minutes)
        const maxAge = 30 * 60 * 1000;
        if (Date.now() - parsed.timestamp > maxAge) {
            sessionStorage.removeItem(STORAGE_KEY);
            showCacheMissMessage(evt.detail.target);
            return;
        }

        // Resubmit the spec to /api/explore
        resubmitSpec(parsed, evt.detail.target, evt.detail.requestConfig);
    });

    // Mode toggle handling (upload vs paste vs url)
    document.addEventListener('click', function(evt) {
        const modeBtn = evt.target.closest('.mode-btn');
        if (!modeBtn) {
            return;
        }

        const container = modeBtn.closest('.input-section');
        if (!container) {
            return;
        }

        const mode = modeBtn.dataset.mode;
        if (!mode) {
            return;
        }

        // Update button active states
        container.querySelectorAll('.mode-btn').forEach(function(btn) {
            btn.classList.toggle('active', btn.dataset.mode === mode);
        });

        // Show/hide content sections
        container.querySelectorAll('.input-content').forEach(function(content) {
            const isActive = content.dataset.mode === mode;
            content.classList.toggle('hidden', !isActive);

            // Enable/disable inputs
            content.querySelectorAll('input, textarea').forEach(function(input) {
                input.disabled = !isActive;
                if (!isActive) {
                    input.removeAttribute('required');
                } else if (input.dataset.required === 'true') {
                    input.setAttribute('required', '');
                }
            });
        });

        // Update hidden mode field
        const modeInput = container.querySelector('input[name$="_mode"]') ||
                          container.querySelector('input[name="input_mode"]');
        if (modeInput) {
            modeInput.value = mode;
        }

        // Clear stored spec when mode changes
        sessionStorage.removeItem(STORAGE_KEY);
    });

    // Tab switching for explore results
    document.addEventListener('click', function(evt) {
        const tabBtn = evt.target.closest('.tab-btn');
        if (!tabBtn) {
            return;
        }

        const tabContainer = tabBtn.closest('.explore-tabs');
        if (!tabContainer) {
            return;
        }

        const tab = tabBtn.dataset.tab;

        // Update button active states
        tabContainer.querySelectorAll('.tab-btn').forEach(function(btn) {
            btn.classList.toggle('active', btn === tabBtn);
        });

        // Show/hide group-by control (only visible for operations tab)
        const tabControls = tabContainer.querySelector('.tab-controls');
        if (tabControls) {
            tabControls.classList.toggle('hidden', tab !== 'operations');
        }
    });

    // Resubmit spec after cache miss
    function resubmitSpec(storedData, target, originalConfig) {
        const formData = new FormData();

        if (storedData.url) {
            // URL mode - set the URL
            formData.append('spec_url', storedData.url);
            formData.append('input_mode', 'url');
        } else if (storedData.content) {
            // File/paste mode - create a blob
            const blob = new Blob([storedData.content], { type: 'text/plain' });
            const filename = storedData.filename || 'spec.yaml';
            formData.append('spec', blob, filename);
            formData.append('input_mode', 'file');
        } else {
            showCacheMissMessage(target);
            return;
        }

        // Show loading state
        const resultsContainer = document.getElementById('explore-results');
        if (resultsContainer) {
            resultsContainer.innerHTML = '<p class="loading-message">Re-uploading specification...</p>';
        }

        fetch('/api/explore', {
            method: 'POST',
            body: formData,
            headers: {
                'HX-Request': 'true'
            }
        })
        .then(function(response) {
            if (!response.ok) {
                throw new Error('Failed to resubmit specification');
            }
            return response.text();
        })
        .then(function(html) {
            // Update the results container
            if (resultsContainer) {
                resultsContainer.innerHTML = html;
                // Process any htmx attributes in the new content
                htmx.process(resultsContainer);
            }

            // Get the new hash from the response and retry the original request
            const hashElement = resultsContainer.querySelector('[data-hash]');
            if (hashElement && originalConfig) {
                const newHash = hashElement.dataset.hash;
                retryOriginalRequest(originalConfig, newHash);
            }
        })
        .catch(function(err) {
            console.error('Failed to resubmit spec:', err);
            showCacheMissMessage(target);
        });
    }

    // Retry the original request with the new hash
    function retryOriginalRequest(originalConfig, newHash) {
        if (!originalConfig || !originalConfig.path) {
            return;
        }

        // Replace the old hash in the URL with the new one
        let newPath = originalConfig.path;
        if (newPath.includes('h=')) {
            newPath = newPath.replace(/h=[^&]+/, 'h=' + newHash);
        } else if (newPath.includes('?')) {
            newPath += '&h=' + newHash;
        } else {
            newPath += '?h=' + newHash;
        }

        // Issue the new request via htmx
        const target = document.querySelector(originalConfig.target || '#tab-content');
        if (target) {
            htmx.ajax('GET', newPath, { target: target, swap: 'innerHTML' });
        }
    }

    // Show cache miss message
    function showCacheMissMessage(target) {
        const message = document.createElement('div');
        message.className = 'cache-miss-message';
        message.innerHTML = '<p>Session expired. Please re-upload your specification.</p>';

        // Clear the explore results and show the message
        const resultsContainer = document.getElementById('explore-results');
        if (resultsContainer) {
            resultsContainer.innerHTML = '';
            resultsContainer.appendChild(message);
        } else if (target) {
            target.innerHTML = '';
            target.appendChild(message);
        }
    }
})();
