"""Shared chrome and original product/audience pages for the public static site."""
from html import escape
import json
from pathlib import Path
import re
from urllib.parse import urlencode

SOURCE = Path(__file__).resolve().parent
SITE_URL = 'https://jagdishjadeja.github.io/pbmanager/'
GITHUB = 'https://github.com/jagdishjadeja/pbmanager'
PRODUCTS = [('managed-pocketbase', 'Managed PocketBase'), ('frontend-hosting', 'Frontend hosting'), ('custom-domains', 'Custom domains'), ('self-hosting', 'Self-hosting')]
AUDIENCES = [('for-developers', 'Developers & freelancers'), ('for-agencies', 'Agencies')]
LABELS = dict(PRODUCTS + AUDIENCES + [('agents', 'Database for agents')])


def icon(name='arrow'):
    return f'<svg aria-hidden="true"><use href="#i-{name}"/></svg>'


def link(prefix, slug, label, active='', class_name=''):
    current = ' aria-current="page"' if slug == active else ''
    css = f' class="{class_name}"' if class_name else ''
    return f'<a{css} href="{prefix}{slug}/"{current}>{escape(label)}</a>'


def header(prefix, active):
    hosting_href = prefix + '#get-started' if active == 'self-hosting' else '#get-started'
    def dropdown(label, items):
        links = ''.join(link(prefix, slug, name, active) for slug, name in items)
        selected = ' data-current-section="true"' if active in dict(items) else ''
        return f'<details class="nav-dropdown"{selected}><summary>{label}<svg aria-hidden="true" viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></svg></summary><div class="nav-dropdown-panel">{links}</div></details>'
    return f'''<header class="site-header container">
<a class="brand" href="{prefix}" aria-label="PBManager home"><span class="brand-icon">{icon('server')}</span>pbmanager</a>
<nav class="site-nav" id="site-nav" aria-label="Main navigation">
{link(prefix, 'agents', 'For agents', active, 'agents-feature')}
{dropdown('Product', PRODUCTS)}{dropdown('Who it’s for', AUDIENCES)}
<a href="{prefix}demo/">Demo</a></nav>
<div class="nav-actions"><a class="source-link" href="{GITHUB}" aria-label="View PBManager on GitHub">{icon('github')}</a><a class="button button-blue nav-button" href="{hosting_href}">Request hosting</a><button class="menu-toggle" aria-label="Open navigation" aria-expanded="false" aria-controls="site-nav"><svg aria-hidden="true" viewBox="0 0 24 24"><path d="M4 7h16M4 12h16M4 17h16"/></svg></button></div></header>'''


def footer(prefix):
    def group(title, items):
        return f'<div><h2>{title}</h2>'+''.join(link(prefix, slug, label) for slug, label in items)+'</div>'
    return f'''<footer class="marketing-footer container"><div class="footer-heading"><a class="brand" href="{prefix}"><span class="brand-icon">{icon('server')}</span>pbmanager</a><p>Built around PocketBase.<br>For the things you and your agents build.</p></div><nav class="footer-directory" aria-label="Footer">
{group('Product', PRODUCTS + [('agents', 'Database for agents')])}{group('Who it’s for', AUDIENCES)}
<div><h2>Explore</h2><a href="{prefix}demo/">Dashboard demo</a><a href="{GITHUB}">GitHub</a><a href="{GITHUB}#deploy-on-your-own-vps">Deployment guide</a><a href="{GITHUB}/issues/new?title=Question%20about%20PBManager">Ask a question</a></div></nav></footer>'''


def decorate(page, route, fonts, styles):
    prefix = './' if route == '' else '../'
    page = page.replace('<!-- SITE_HEADER -->', header(prefix, route))
    page = page.replace('<!-- SITE_FOOTER -->', footer(prefix))
    page = page.replace('/* SITE_NAVIGATION */', (SOURCE / 'navigation.js').read_text())
    page = page.replace('/* FONT_FACES */', fonts).replace('/* SITE_STYLES */', styles)
    canonical = SITE_URL + (route + '/' if route else '')
    if '<link rel="canonical"' not in page:
        page = page.replace('</head>', f'<link rel="canonical" href="{canonical}">\n</head>', 1)
    return page


def product_page(data, shell):
    slug = data['slug']
    title = data['title'] + ' · PBManager'
    description = escape(data['description'], quote=True)
    head = shell.split('<main id="main">')[0]
    head = re.sub(r'<title>.*?</title>', f'<title>{escape(title)}</title>', head)
    for name, value in [('name="description"', description), ('property="og:title"', escape(title, quote=True)), ('property="og:description"', description), ('property="og:url"', SITE_URL + slug + '/')]:
        head = re.sub(r'<meta ' + name + r' content="[^"]*">', f'<meta {name} content="{value}">', head)
    heading = escape(data['heading'][0]) + '<span>' + escape(data['heading'][1]) + '</span>'
    request = GITHUB + '/issues/new?' + urlencode({'title': data['title'] + ' request', 'body': 'Hi! I am interested in ' + data['title'].lower() + '.\n\nAbout my project:\n\nExpected storage and traffic:\n\nQuestions:\n'})
    primary_href = data.get('primary_href', '#get-started')
    primary_label = data.get('primary_label', 'Discuss your project')
    proof = ''.join(f'<div><dt>{escape(a)}</dt><dd>{escape(b)}</dd></div>' for a,b in data['proof_rows'])
    benefits = ''.join(f'<article class="feature-row">{icon(name)}<div><h3>{escape(a)}</h3><p>{escape(b)}</p></div></article>' for (a,b),name in zip(data['benefits'], ['server','chart','globe']))
    steps = ''.join(f'<li><h3>{escape(a)}</h3><p>{escape(b)}</p></li>' for a,b in data['steps'])
    faq = ''.join(f'<details{ " open" if i == 0 else ""}><summary>{escape(a)}{icon("plus")}</summary><p>{escape(b)}</p></details>' for i,(a,b) in enumerate(data['faq']))
    related = ''.join(f'<a href="../{key}/">{escape(LABELS[key])}{icon()}</a>' for key in data['related'])
    closing_href = data.get('primary_href', request)
    closing_label = data.get('primary_label', 'Request hosting access')
    closing_note = 'Build and installation instructions on GitHub.' if slug == 'self-hosting' else 'Opens a request on GitHub. Pricing confirmed individually.'
    benefit_title = '<br>'.join(escape(data['benefit_title']).split('\n'))
    return head + f'''<main id="main">
<section class="hero product-hero" aria-labelledby="hero-title"><div class="hero-inner container"><div class="hero-copy"><h1 id="hero-title">{heading}</h1><p class="hero-description">{description}</p><div class="hero-actions"><a class="button button-white" href="{escape(primary_href, quote=True)}">{escape(primary_label)}{icon()}</a><a class="hero-demo" href="#how-it-works">How it works{icon()}</a></div></div><aside class="product-proof" aria-labelledby="proof-title"><h2 id="proof-title">{escape(data['proof_title'])}</h2><dl>{proof}</dl><p>{escape(data['proof_note'])}</p></aside></div></section>
<section class="features container" aria-labelledby="benefits-title"><div class="features-heading"><h2 id="benefits-title">{benefit_title}</h2><p>{escape(data['benefit_intro'])}</p></div><div class="feature-list">{benefits}</div></section>
<section class="product-process" id="how-it-works" aria-labelledby="process-title"><div class="container"><h2 id="process-title">{escape(data['steps_title'])}</h2><ol class="process-steps">{steps}</ol><p class="product-boundary">{escape(data['note'])}</p></div></section>
<section class="faq container" id="questions" aria-labelledby="faq-title"><div class="faq-intro"><h2 id="faq-title">Before you<br>get started.</h2><p>Details to help you choose the right home for your project.</p><a class="text-link" href="../demo/">Explore the dashboard{icon('external')}</a></div><div>{faq}</div></section>
<section class="related-pages container" aria-labelledby="related-title"><h2 id="related-title">Keep exploring.</h2><nav aria-label="Related pages">{related}</nav></section>
<section class="closing" id="get-started" aria-labelledby="closing-title"><div class="closing-inner container"><div><h2 id="closing-title">{escape(data['closing'])}</h2><p>{escape(data['closing_description'])}</p></div><div class="closing-actions"><a class="button button-white" href="{escape(closing_href, quote=True)}">{escape(closing_label)}{icon()}</a><span>{closing_note}</span></div></div></section>
</main><!-- SITE_FOOTER --><script>/* SITE_NAVIGATION */</script></body></html>'''


def pages():
    return json.loads((SOURCE / 'marketing-pages.json').read_text())
