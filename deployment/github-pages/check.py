#!/usr/bin/env python3
"""Validate generated marketing routes and shared navigation before publication."""
from html.parser import HTMLParser
from pathlib import Path
import re
import subprocess
import tempfile
from urllib.parse import unquote, urlsplit
import xml.etree.ElementTree as ET
from marketing import pages, SITE_URL

ROOT = Path(__file__).resolve().parents[2]
DOCS = ROOT / 'docs'

class Document(HTMLParser):
    def __init__(self):
        super().__init__()
        self.ids = []
        self.links = []
        self.h1_count = 0
        self.main_count = 0
        self.canonical = []
        self.current = []
        self.description = None
        self.featured = 0
    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if 'id' in attrs: self.ids.append(attrs['id'])
        if tag == 'h1': self.h1_count += 1
        if tag == 'main': self.main_count += 1
        if tag == 'a':
            self.links.append(attrs.get('href', ''))
            if attrs.get('aria-current') == 'page': self.current.append(attrs['href'])
            if 'agents-feature' in attrs.get('class', '').split(): self.featured += 1
        if tag == 'link' and attrs.get('rel') == 'canonical': self.canonical.append(attrs['href'])
        if tag == 'meta' and attrs.get('name') == 'description': self.description = attrs.get('content')


def main():
    routes = ['', 'agents'] + [item['slug'] for item in pages()]
    titles = set()
    link_count = 0
    for route in routes:
        file = DOCS / route / 'index.html'
        html = file.read_text()
        document = Document()
        document.feed(html)
        assert document.h1_count == document.main_count == 1, file
        assert len(document.ids) == len(set(document.ids)), f'Duplicate IDs in {file}'
        assert document.featured == 1, f'Agents highlight missing in {file}'
        assert document.description, f'Description missing in {file}'
        canonical = SITE_URL + (route + '/' if route else '')
        assert document.canonical == [canonical], file
        assert len(document.current) == (1 if route else 0), file
        for marker in ['<!-- SITE_HEADER -->', '<!-- SITE_FOOTER -->', '/* SITE_NAVIGATION */', '/* SITE_STYLES */', '/* FONT_FACES */']:
            assert marker not in html, f'Unexpanded {marker} in {file}'
        title = re.search(r'<title>(.*?)</title>', html).group(1)
        assert title not in titles, f'Duplicate title: {title}'
        titles.add(title)
        for href in document.links:
            parsed = urlsplit(href)
            if parsed.scheme or parsed.netloc: continue
            target = (file.parent / unquote(parsed.path)).resolve() if parsed.path else file
            assert target.is_relative_to(DOCS), href
            if target.is_dir(): target /= 'index.html'
            assert target.is_file(), f'{file}: broken link {href}'
            if parsed.fragment:
                target_doc = Document()
                target_doc.feed(target.read_text())
                assert parsed.fragment in target_doc.ids, f'{file}: missing anchor {href}'
            link_count += 1
        for script in re.findall(r'<script\b[^>]*>(.*?)</script>', html, re.S):
            with tempfile.NamedTemporaryFile(mode='w', suffix='.js') as js:
                js.write(script)
                js.flush()
                subprocess.run(['node', '--check', js.name], check=True, capture_output=True)
    sitemap = ET.parse(DOCS / 'sitemap.xml')
    urls = {node.text for node in sitemap.findall('.//{http://www.sitemaps.org/schemas/sitemap/0.9}loc')}
    assert urls == {SITE_URL + (route + '/' if route else '') for route in routes}
    print(f'PASS: {len(routes)} pages, {link_count} local links, unique titles and IDs, canonical URLs, sitemap, active navigation, featured agents link, and JavaScript syntax.')

if __name__ == '__main__':
    main()
