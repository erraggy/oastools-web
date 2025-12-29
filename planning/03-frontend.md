# Frontend Implementation

## Overview

The oastools-web frontend follows a minimal, server-rendered architecture using HTMX for dynamic interactions and Go's `html/template` package for rendering. This approach eliminates build steps, npm dependencies, and JavaScript framework complexity while providing a responsive user experience.

## Technology Stack

The frontend consists of HTMX (14kb gzipped) loaded from CDN for dynamic page updates without full page reloads, Go's `html/template` for server-side rendering with automatic HTML escaping, and minimal custom CSS for styling. No JavaScript build process, bundler, or package manager is required.

## Page Structure

### Base Template

The base template establishes the document structure, loads HTMX, and defines the layout framework.

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - oastools</title>
    <script src="https://unpkg.com/htmx.org@2.0.0"></script>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <header>
        <nav>
            <a href="/" class="logo">oastools</a>
            <ul>
                <li><a href="/validate">Validate</a></li>
                <li><a href="/convert">Convert</a></li>
                <li><a href="/diff">Diff</a></li>
                <li><a href="/fix">Fix</a></li>
                <li><a href="/join">Join</a></li>
            </ul>
            <a href="https://github.com/erraggy/oastools" class="github" target="_blank">GitHub</a>
        </nav>
    </header>
    
    <main>
        {{block "content" .}}{{end}}
    </main>
    
    <footer>
        <p>Powered by <a href="https://github.com/erraggy/oastools">oastools</a> v{{.OASToolsVersion}}</p>
    </footer>
    
    {{block "scripts" .}}{{end}}
</body>
</html>
```

### Landing Page

The landing page presents the available operations as cards, each linking to its dedicated page.

```html
{{define "content"}}
<section class="hero">
    <h1>OpenAPI Toolkit</h1>
    <p>Validate, convert, diff, fix, and join OpenAPI specifications in your browser.</p>
</section>

<section class="operations">
    <article class="operation-card">
        <h2>Validate</h2>
        <p>Check your OpenAPI spec for errors and warnings</p>
        <a href="/validate" class="button">Try it</a>
    </article>
    
    <article class="operation-card">
        <h2>Convert</h2>
        <p>Convert between OpenAPI 2.0, 3.0, 3.1, and 3.2</p>
        <a href="/convert" class="button">Try it</a>
    </article>
    
    <article class="operation-card">
        <h2>Diff</h2>
        <p>Compare two specifications and see what changed</p>
        <a href="/diff" class="button">Try it</a>
    </article>
    
    <article class="operation-card">
        <h2>Fix</h2>
        <p>Automatically fix common issues in your spec</p>
        <a href="/fix" class="button">Try it</a>
    </article>
    
    <article class="operation-card">
        <h2>Join</h2>
        <p>Merge multiple specifications into one</p>
        <a href="/join" class="button">Try it</a>
    </article>
</section>
{{end}}
```

## Operation Pages

Each operation has a dedicated page with an appropriate form and results area.

### Validate Page

The validate page demonstrates the basic single-file upload pattern used by validate, fix, and convert operations.

```html
{{define "content"}}
<section class="operation">
    <h1>Validate OpenAPI Specification</h1>
    <p>Upload your OpenAPI specification to check for errors and warnings.</p>
    
    <form hx-post="/api/validate" 
          hx-target="#results" 
          hx-swap="innerHTML"
          hx-encoding="multipart/form-data"
          hx-indicator="#loading">
        
        <div class="file-upload">
            <label for="spec">OpenAPI Specification</label>
            <input type="file" 
                   id="spec" 
                   name="spec" 
                   accept=".json,.yaml,.yml"
                   required>
            <span class="hint">JSON or YAML, max 2MB</span>
        </div>
        
        <button type="submit" class="button primary">Validate</button>
    </form>
    
    <div id="loading" class="htmx-indicator">
        <span class="spinner"></span> Processing...
    </div>
    
    <section id="results" class="results">
        <!-- Results injected here by HTMX -->
    </section>
</section>
{{end}}
```

### Convert Page

The convert page adds a target version selector.

```html
{{define "content"}}
<section class="operation">
    <h1>Convert OpenAPI Specification</h1>
    <p>Convert your specification between OpenAPI versions.</p>
    
    <form hx-post="/api/convert" 
          hx-target="#results" 
          hx-swap="innerHTML"
          hx-encoding="multipart/form-data"
          hx-indicator="#loading">
        
        <div class="file-upload">
            <label for="spec">OpenAPI Specification</label>
            <input type="file" 
                   id="spec" 
                   name="spec" 
                   accept=".json,.yaml,.yml"
                   required>
        </div>
        
        <div class="form-group">
            <label for="target">Target Version</label>
            <select id="target" name="target" required>
                <option value="2.0">OpenAPI 2.0 (Swagger)</option>
                <option value="3.0">OpenAPI 3.0</option>
                <option value="3.1" selected>OpenAPI 3.1</option>
                <option value="3.2">OpenAPI 3.2</option>
            </select>
        </div>
        
        <button type="submit" class="button primary">Convert</button>
    </form>
    
    <div id="loading" class="htmx-indicator">
        <span class="spinner"></span> Converting...
    </div>
    
    <section id="results" class="results"></section>
</section>
{{end}}
```

### Diff Page

The diff page requires two file inputs for the base and head specifications.

```html
{{define "content"}}
<section class="operation">
    <h1>Diff OpenAPI Specifications</h1>
    <p>Compare two specifications and see what changed between them.</p>
    
    <form hx-post="/api/diff" 
          hx-target="#results" 
          hx-swap="innerHTML"
          hx-encoding="multipart/form-data"
          hx-indicator="#loading">
        
        <div class="file-upload-group">
            <div class="file-upload">
                <label for="base">Base Specification (before)</label>
                <input type="file" 
                       id="base" 
                       name="base" 
                       accept=".json,.yaml,.yml"
                       required>
            </div>
            
            <div class="file-upload">
                <label for="head">Head Specification (after)</label>
                <input type="file" 
                       id="head" 
                       name="head" 
                       accept=".json,.yaml,.yml"
                       required>
            </div>
        </div>
        
        <button type="submit" class="button primary">Compare</button>
    </form>
    
    <div id="loading" class="htmx-indicator">
        <span class="spinner"></span> Comparing...
    </div>
    
    <section id="results" class="results"></section>
</section>
{{end}}
```

### Join Page

The join page supports multiple file uploads with dynamic file input addition.

```html
{{define "content"}}
<section class="operation">
    <h1>Join OpenAPI Specifications</h1>
    <p>Merge multiple specifications into a single document.</p>
    
    <form hx-post="/api/join" 
          hx-target="#results" 
          hx-swap="innerHTML"
          hx-encoding="multipart/form-data"
          hx-indicator="#loading"
          id="join-form">
        
        <div id="file-inputs">
            <div class="file-upload">
                <label>Specification 1</label>
                <input type="file" name="spec[]" accept=".json,.yaml,.yml" required>
            </div>
            <div class="file-upload">
                <label>Specification 2</label>
                <input type="file" name="spec[]" accept=".json,.yaml,.yml" required>
            </div>
        </div>
        
        <button type="button" id="add-file" class="button secondary">
            + Add Another File
        </button>
        
        <div class="form-group">
            <label for="collisionStrategy">Collision Strategy</label>
            <select id="collisionStrategy" name="collisionStrategy">
                <option value="rename" selected>Rename (append suffix)</option>
                <option value="first">Keep First</option>
                <option value="error">Error on Collision</option>
            </select>
        </div>
        
        <button type="submit" class="button primary">Join</button>
    </form>
    
    <div id="loading" class="htmx-indicator">
        <span class="spinner"></span> Merging...
    </div>
    
    <section id="results" class="results"></section>
</section>
{{end}}

{{define "scripts"}}
<script>
document.getElementById('add-file').addEventListener('click', function() {
    const container = document.getElementById('file-inputs');
    const count = container.querySelectorAll('input[type="file"]').length;
    
    if (count >= 5) {
        alert('Maximum 5 files allowed');
        return;
    }
    
    const div = document.createElement('div');
    div.className = 'file-upload';
    div.innerHTML = `
        <label>Specification ${count + 1}</label>
        <input type="file" name="spec[]" accept=".json,.yaml,.yml" required>
        <button type="button" class="remove-file" onclick="this.parentElement.remove()">×</button>
    `;
    container.appendChild(div);
});
</script>
{{end}}
```

## Result Partials

Result partials render the output of each operation. These are designed to work both as fragments (for HTMX swaps) and within full page renders.

### Validation Result Partial

```html
{{define "validation-result"}}
<div class="validation-result {{if .Valid}}valid{{else}}invalid{{end}}">
    <header class="result-header">
        <h2>{{if .Valid}}✓ Valid{{else}}✗ Invalid{{end}}</h2>
        <span class="version">OpenAPI {{.Version}}</span>
    </header>
    
    <dl class="statistics">
        <div><dt>Paths</dt><dd>{{.Statistics.Paths}}</dd></div>
        <div><dt>Operations</dt><dd>{{.Statistics.Operations}}</dd></div>
        <div><dt>Schemas</dt><dd>{{.Statistics.Schemas}}</dd></div>
        <div><dt>Errors</dt><dd class="{{if gt .Statistics.Errors 0}}error{{end}}">{{.Statistics.Errors}}</dd></div>
        <div><dt>Warnings</dt><dd class="{{if gt .Statistics.Warnings 0}}warning{{end}}">{{.Statistics.Warnings}}</dd></div>
    </dl>
    
    {{if .Errors}}
    <section class="issues errors">
        <h3>Errors</h3>
        <ul>
            {{range .Errors}}
            <li>
                <code class="path">{{.Path}}</code>
                <span class="message">{{.Message}}</span>
            </li>
            {{end}}
        </ul>
    </section>
    {{end}}
    
    {{if .Warnings}}
    <section class="issues warnings">
        <h3>Warnings</h3>
        <ul>
            {{range .Warnings}}
            <li>
                <code class="path">{{.Path}}</code>
                <span class="message">{{.Message}}</span>
            </li>
            {{end}}
        </ul>
    </section>
    {{end}}
</div>
{{end}}
```

### Conversion Result Partial

```html
{{define "conversion-result"}}
<div class="conversion-result">
    <header class="result-header">
        <h2>Conversion Complete</h2>
        <span class="version">{{.SourceVersion}} → {{.TargetVersion}}</span>
    </header>
    
    {{if .Issues}}
    <section class="issues">
        <h3>Conversion Notes</h3>
        <ul>
            {{range .Issues}}
            <li class="{{.Severity}}">
                <code class="path">{{.Path}}</code>
                <span class="message">{{.Message}}</span>
            </li>
            {{end}}
        </ul>
    </section>
    {{end}}
    
    <section class="result-output">
        <header>
            <h3>Result</h3>
            <button class="button small" onclick="downloadResult()">Download</button>
            <button class="button small" onclick="copyResult()">Copy</button>
        </header>
        <pre><code id="result-code">{{.Result}}</code></pre>
    </section>
</div>

<script>
function downloadResult() {
    const content = document.getElementById('result-code').textContent;
    const format = '{{.Format}}';
    const ext = format === 'json' ? 'json' : 'yaml';
    const blob = new Blob([content], {type: 'text/' + format});
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'converted.' + ext;
    a.click();
    URL.revokeObjectURL(url);
}

function copyResult() {
    const content = document.getElementById('result-code').textContent;
    navigator.clipboard.writeText(content);
}
</script>
{{end}}
```

### Diff Result Partial

```html
{{define "diff-result"}}
<div class="diff-result">
    <header class="result-header">
        <h2>Comparison Results</h2>
    </header>
    
    <dl class="summary">
        <div class="addition"><dt>Additions</dt><dd>{{.Summary.Additions}}</dd></div>
        <div class="deletion"><dt>Deletions</dt><dd>{{.Summary.Deletions}}</dd></div>
        <div class="modification"><dt>Modifications</dt><dd>{{.Summary.Modifications}}</dd></div>
        <div class="breaking"><dt>Breaking Changes</dt><dd>{{.Summary.Breaking}}</dd></div>
    </dl>
    
    {{if .Changes}}
    <section class="changes">
        <h3>Changes</h3>
        <table>
            <thead>
                <tr>
                    <th>Type</th>
                    <th>Path</th>
                    <th>Description</th>
                    <th>Breaking</th>
                </tr>
            </thead>
            <tbody>
                {{range .Changes}}
                <tr class="{{.Type}} {{if .Breaking}}breaking{{end}}">
                    <td><span class="badge {{.Type}}">{{.Type}}</span></td>
                    <td><code>{{.Path}}</code></td>
                    <td>{{.Description}}</td>
                    <td>{{if .Breaking}}⚠️{{else}}—{{end}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </section>
    {{else}}
    <p class="no-changes">No differences found between the specifications.</p>
    {{end}}
</div>
{{end}}
```

## Styling

The CSS follows a minimal approach, using system fonts, reasonable spacing, and accessible color contrasts.

```css
/* Base styles */
:root {
    --color-primary: #2563eb;
    --color-success: #16a34a;
    --color-warning: #ca8a04;
    --color-error: #dc2626;
    --color-text: #1f2937;
    --color-muted: #6b7280;
    --color-border: #e5e7eb;
    --color-bg: #f9fafb;
    --font-mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace;
}

body {
    font-family: system-ui, -apple-system, sans-serif;
    line-height: 1.6;
    color: var(--color-text);
    max-width: 1200px;
    margin: 0 auto;
    padding: 1rem;
}

/* Navigation */
nav {
    display: flex;
    align-items: center;
    gap: 2rem;
    padding: 1rem 0;
    border-bottom: 1px solid var(--color-border);
}

nav ul {
    display: flex;
    gap: 1.5rem;
    list-style: none;
    margin: 0;
    padding: 0;
}

/* Forms */
.file-upload {
    margin-bottom: 1rem;
}

.file-upload label {
    display: block;
    font-weight: 500;
    margin-bottom: 0.5rem;
}

.file-upload input[type="file"] {
    display: block;
    width: 100%;
    padding: 0.75rem;
    border: 2px dashed var(--color-border);
    border-radius: 0.5rem;
    background: var(--color-bg);
}

.button {
    display: inline-block;
    padding: 0.75rem 1.5rem;
    border: none;
    border-radius: 0.375rem;
    font-weight: 500;
    cursor: pointer;
    text-decoration: none;
}

.button.primary {
    background: var(--color-primary);
    color: white;
}

/* Results */
.results {
    margin-top: 2rem;
}

.validation-result.valid .result-header {
    border-left: 4px solid var(--color-success);
}

.validation-result.invalid .result-header {
    border-left: 4px solid var(--color-error);
}

.issues ul {
    list-style: none;
    padding: 0;
}

.issues li {
    padding: 0.75rem;
    margin-bottom: 0.5rem;
    background: var(--color-bg);
    border-radius: 0.375rem;
}

.issues .path {
    display: block;
    font-family: var(--font-mono);
    font-size: 0.875rem;
    color: var(--color-muted);
}

/* Loading indicator */
.htmx-indicator {
    display: none;
}

.htmx-request .htmx-indicator {
    display: block;
}

.spinner {
    display: inline-block;
    width: 1rem;
    height: 1rem;
    border: 2px solid var(--color-border);
    border-top-color: var(--color-primary);
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
}

@keyframes spin {
    to { transform: rotate(360deg); }
}
```

## Error Handling

When an API request fails, the server returns an error partial that HTMX swaps into the results area.

```html
{{define "error"}}
<div class="error-result">
    <h2>Error</h2>
    <p class="error-message">{{.Message}}</p>
    {{if .Details}}
    <details>
        <summary>Details</summary>
        <pre>{{.Details}}</pre>
    </details>
    {{end}}
</div>
{{end}}
```

## Progress Feedback

For operations that may take several seconds, the HTMX indicator provides visual feedback.

```html
<form hx-post="/api/validate" 
      hx-target="#results" 
      hx-indicator="#loading">
    <!-- form fields -->
</form>

<div id="loading" class="htmx-indicator">
    <span class="spinner"></span>
    <span>Processing your specification...</span>
</div>
```

The CSS rule `.htmx-request .htmx-indicator { display: block; }` shows the indicator while a request is in flight.

## Accessibility Considerations

The frontend implements several accessibility best practices. All form inputs have associated labels. Color is not the only indicator of state (icons accompany status colors). The focus states are visible for keyboard navigation. Semantic HTML elements (header, nav, main, section) structure the content. ARIA attributes are used where appropriate, such as `aria-live="polite"` on the results container to announce updates to screen readers.

## Browser Support

The frontend targets modern browsers (Chrome, Firefox, Safari, Edge from the last 2 years). HTMX 2.0 requires no polyfills for these browsers. The CSS uses widely supported features with no vendor prefixes required.
