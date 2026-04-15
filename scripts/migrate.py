#!/usr/bin/env python3
"""
migrate.py — Génère/met à jour bookmarks.csv depuis l'historique git + bookmarks.html

Usage:
    python3 scripts/migrate.py

Règles:
- Ne jamais écraser une date déjà dans le CSV
- Utiliser la date du commit git quand l'URL a été ajoutée
- Fallback: ADD_DATE de l'attribut HTML si absent du git history
- Tri final par date_added ASC
"""

import subprocess
import csv
import re
import os
import sys
from html.parser import HTMLParser
from datetime import datetime, timezone

REPO_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BOOKMARKS_HTML = os.path.join(REPO_DIR, 'bookmarks.html')
BOOKMARKS_CSV = os.path.join(REPO_DIR, 'bookmarks.csv')
CSV_HEADERS = ['url', 'name', 'category', 'subcategory', 'description', 'tags', 'date_added']


# ─── Parser HTML ──────────────────────────────────────────────────────────────

class BookmarkParser(HTMLParser):
    """Parse bookmarks.html et retourne une liste de dicts."""

    def __init__(self):
        super().__init__()
        self.bookmarks = []
        self._path_stack = []   # chemin catégories courant
        self._in_a = False
        self._current_a = None

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        tag = tag.upper()
        if tag == 'H3':
            name = attrs.get('add_date', '')  # sera remplacé par le texte
            self._path_stack.append({'name': '', 'add_date': attrs.get('add_date', '0')})
        elif tag == 'A':
            self._in_a = True
            self._current_a = {
                'url': attrs.get('href', ''),
                'add_date': attrs.get('add_date', '0'),
                'name': '',
            }
        elif tag == 'DL':
            pass  # ouverture d'un sous-niveau, géré via le stack

    def handle_endtag(self, tag):
        tag = tag.upper()
        if tag == 'A':
            self._in_a = False
            if self._current_a and self._current_a['url']:
                path = [p['name'] for p in self._path_stack if p['name']]
                self._current_a['category'] = path[0] if len(path) > 0 else ''
                self._current_a['subcategory'] = path[1] if len(path) > 1 else ''
                self.bookmarks.append(self._current_a)
            self._current_a = None
        elif tag == 'DL':
            if self._path_stack:
                self._path_stack.pop()

    def handle_data(self, data):
        data = data.strip()
        if not data:
            return
        if self._in_a and self._current_a:
            self._current_a['name'] += data
        elif self._path_stack:
            # Nom du dossier courant
            self._path_stack[-1]['name'] = data


def parse_bookmarks_html(path):
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    parser = BookmarkParser()
    parser.feed(content)
    return parser.bookmarks


# ─── Git history ──────────────────────────────────────────────────────────────

def get_git_commits(repo_dir, filename='bookmarks.html'):
    """Retourne la liste des commits (oldest first) qui touchent bookmarks.html."""
    result = subprocess.run(
        ['git', 'log', '--reverse', '--pretty=format:%H\t%ai', '--', filename],
        capture_output=True, text=True, cwd=repo_dir
    )
    if result.returncode != 0:
        print(f"[ERROR] git log failed: {result.stderr}", file=sys.stderr)
        return []

    commits = []
    for line in result.stdout.strip().split('\n'):
        if not line.strip():
            continue
        parts = line.split('\t')
        if len(parts) == 2:
            commit_hash, date_str = parts
            dt = datetime.fromisoformat(date_str)
            commits.append({'hash': commit_hash, 'date': dt})
    return commits


def get_urls_added_in_commit(repo_dir, commit_hash, filename='bookmarks.html'):
    """Retourne les URLs des lignes ajoutées dans ce commit."""
    result = subprocess.run(
        ['git', 'diff', f'{commit_hash}^', commit_hash, '--', filename],
        capture_output=True, text=True, cwd=repo_dir
    )
    if result.returncode != 0:
        # Premier commit sans parent — utiliser show
        result = subprocess.run(
            ['git', 'show', f'{commit_hash}:{filename}'],
            capture_output=True, text=True, cwd=repo_dir
        )
        if result.returncode != 0:
            return []
        # Toutes les URLs du fichier au 1er commit
        return re.findall(r'<A HREF="([^"]+)"', result.stdout)

    # Lignes ajoutées (+) contenant une balise <A HREF
    added = re.findall(r'^\+[^+].*<A HREF="([^"]+)"', result.stdout, re.MULTILINE)
    return added


# ─── CSV I/O ──────────────────────────────────────────────────────────────────

def load_csv(path):
    """Charge le CSV existant, retourne un dict {url: row}."""
    if not os.path.exists(path):
        return {}
    data = {}
    with open(path, 'r', encoding='utf-8', newline='') as f:
        reader = csv.DictReader(f)
        for row in reader:
            data[row['url']] = row
    return data


def save_csv(path, data_dict):
    """Écrit le CSV trié par date_added ASC."""
    rows = sorted(data_dict.values(), key=lambda r: int(r.get('date_added') or 0))
    with open(path, 'w', encoding='utf-8', newline='') as f:
        writer = csv.DictWriter(f, fieldnames=CSV_HEADERS)
        writer.writeheader()
        writer.writerows(rows)
    print(f"[OK] {len(rows)} entrées écrites dans {os.path.basename(path)}")


# ─── Main ─────────────────────────────────────────────────────────────────────

def main():
    print(f"[→] Dépôt : {REPO_DIR}")
    print(f"[→] Fichier HTML : {BOOKMARKS_HTML}")
    print(f"[→] Fichier CSV  : {BOOKMARKS_CSV}")
    print()

    # 1. Charger le CSV existant
    existing = load_csv(BOOKMARKS_CSV)
    print(f"[i] {len(existing)} entrées déjà dans le CSV")

    # 2. Parser bookmarks.html pour avoir titre/catégorie/add_date HTML
    html_bookmarks = parse_bookmarks_html(BOOKMARKS_HTML)
    html_meta = {b['url']: b for b in html_bookmarks}
    print(f"[i] {len(html_meta)} URLs trouvées dans bookmarks.html")

    # 3. Récupérer l'historique git
    commits = get_git_commits(REPO_DIR)
    print(f"[i] {len(commits)} commits touchant bookmarks.html")

    added_count = 0
    skipped_count = 0

    # 4. Parcourir les commits (oldest first) pour dater les URLs
    for commit in commits:
        date_ts = int(commit['date'].astimezone(timezone.utc).timestamp())
        urls = get_urls_added_in_commit(REPO_DIR, commit['hash'])

        for url in urls:
            if not url or url.startswith('#'):
                continue
            if url in existing:
                # Ne jamais écraser
                skipped_count += 1
                continue

            meta = html_meta.get(url, {})
            existing[url] = {
                'url': url,
                'name': meta.get('name', ''),
                'category': meta.get('category', ''),
                'subcategory': meta.get('subcategory', ''),
                'description': '',
                'tags': '',
                'date_added': date_ts,
            }
            added_count += 1

    print(f"[i] {added_count} nouvelles URLs datées depuis git history")
    print(f"[i] {skipped_count} URLs déjà présentes → non modifiées")

    # 5. Fallback pour URLs dans HTML mais absentes du git diff
    #    (ex: URLs retirées + réajoutées, ou absentes du diff)
    fallback_count = 0
    for url, meta in html_meta.items():
        if url not in existing:
            add_date_html = int(meta.get('add_date', '0') or '0')
            existing[url] = {
                'url': url,
                'name': meta.get('name', ''),
                'category': meta.get('category', ''),
                'subcategory': meta.get('subcategory', ''),
                'description': '',
                'tags': '',
                'date_added': add_date_html,
            }
            fallback_count += 1

    if fallback_count:
        print(f"[i] {fallback_count} URLs avec fallback ADD_DATE HTML")

    # 6. Sauvegarder
    print()
    save_csv(BOOKMARKS_CSV, existing)

    # 7. Résumé
    print()
    print("=== Résumé ===")
    print(f"Total dans CSV : {len(existing)}")
    dated = sum(1 for r in existing.values() if int(r.get('date_added') or 0) > 0)
    print(f"Avec date      : {dated}")
    print(f"Sans date      : {len(existing) - dated}")


if __name__ == '__main__':
    main()
