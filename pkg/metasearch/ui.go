package metasearch

import (
	"html/template"
	"os"
	"path/filepath"
)

const resultsTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Query}} - iTaK Search</title>
    <style>
        :root {
            --bg-color: #0f111a;
            --surface-color: #1a1d2d;
            --text-main: #e2e8f0;
            --text-muted: #94a3b8;
            --accent-color: #38bdf8;
            --border-color: #334155;
            --hover-color: #1e293b;
            --wiki-color: #facc15;
            --ddg-color: #fd5c63;
        }
        
        body {
            font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
            background-color: var(--bg-color);
            color: var(--text-main);
            margin: 0;
            padding: 0;
            line-height: 1.6;
        }
        
        .header {
            background-color: var(--surface-color);
            padding: 20px 40px;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            align-items: center;
            gap: 20px;
            position: sticky;
            top: 0;
            z-index: 100;
            box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
        }
        
        .logo {
            font-size: 24px;
            font-weight: 800;
            color: var(--text-main);
            text-decoration: none;
            letter-spacing: -0.5px;
        }
        .logo span {
            color: var(--accent-color);
        }
        
        .search-container {
            flex-grow: 1;
            max-width: 600px;
        }
        
        .search-box {
            width: 100%;
            padding: 12px 20px;
            border-radius: 24px;
            border: 1px solid var(--border-color);
            background-color: var(--bg-color);
            color: var(--text-main);
            font-size: 16px;
            outline: none;
            transition: border-color 0.2s, box-shadow 0.2s;
        }
        
        .search-box:focus {
            border-color: var(--accent-color);
            box-shadow: 0 0 0 2px rgba(56, 189, 248, 0.2);
        }
        
        .container {
            max-width: 800px;
            margin: 0 auto;
            padding: 40px 20px;
        }
        
        .meta-stats {
            color: var(--text-muted);
            font-size: 14px;
            margin-bottom: 24px;
        }
        
        .result-card {
            background: var(--surface-color);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            padding: 20px;
            margin-bottom: 20px;
            transition: transform 0.2s, border-color 0.2s;
            display: flex;
            flex-direction: column;
            gap: 6px;
        }
        
        .result-card:hover {
            border-color: var(--accent-color);
            transform: translateY(-2px);
            box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1);
        }
        
        .source-badge {
            align-self: flex-start;
            font-size: 11px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            font-weight: 700;
            padding: 4px 10px;
            border-radius: 12px;
            background: var(--hover-color);
            color: var(--text-muted);
        }
        .source-wikipedia { color: #111; background: var(--wiki-color); box-shadow: 0 0 8px rgba(250, 204, 21, 0.3); }
        .source-duckduckgo { color: #fff; background: var(--ddg-color); box-shadow: 0 0 8px rgba(253, 92, 99, 0.3); }
        
        .result-url {
            color: var(--text-muted);
            font-size: 13px;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }
        
        .result-title {
            color: var(--accent-color);
            font-size: 20px;
            font-weight: 600;
            text-decoration: none;
        }
        
        .result-title:hover {
            text-decoration: underline;
        }
        
        .result-snippet {
            color: var(--text-main);
            font-size: 15px;
            line-height: 1.5;
            margin-top: 4px;
        }
        
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: var(--text-muted);
        }
    </style>
</head>
<body>

<div class="header">
    <a href="#" class="logo">iTaK<span>Search</span></a>
    <div class="search-container">
        <!-- The CLI handles new searches -->
        <input type="text" class="search-box" value="{{.Query}}" readonly onclick="this.select()">
    </div>
</div>

<div class="container">
    <div class="meta-stats">
        ⚡ Engine aggregated {{len .Results}} results at native speeds
    </div>
    
    {{if not .Results}}
    <div class="empty-state">
        <h2>No results found</h2>
        <p>Try adjusting your search query.</p>
    </div>
    {{else}}
        {{range .Results}}
        <div class="result-card">
            <span class="source-badge source-{{tolower .Source}}">{{.Source}}</span>
            <a href="{{.URL}}" class="result-title">{{.Title}}</a>
            <span class="result-url">{{.URL}}</span>
            <div class="result-snippet">{{.Snippet}}</div>
        </div>
        {{end}}
    {{end}}
</div>

</body>
</html>
`

type templateData struct {
	Query   string
	Results []SearchResult
}

// GenerateLocalHTML generates the UI and returns the absolute file path to the HTML file
func GenerateLocalHTML(query string, results []SearchResult) (string, error) {
	tempDir := os.TempDir()
	filePath := filepath.Join(tempDir, "itak_search_results.html")

	f, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	funcs := template.FuncMap{
		"tolower": func(s string) string {
			return stringToLower(s)
		},
	}

	tmpl, err := template.New("results").Funcs(funcs).Parse(resultsTemplate)
	if err != nil {
		return "", err
	}

	data := templateData{
		Query:   query,
		Results: results,
	}

	err = tmpl.Execute(f, data)
	if err != nil {
		return "", err
	}

	return filePath, nil
}

func stringToLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}
