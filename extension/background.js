// background.js — service worker MV3
// Poll Nitter RSS pour 3 comptes X, stocke dans chrome.storage.local

const POLL_INTERVAL_HOURS = 4;

const ACCOUNTS = [
  { handle: 'bearstech',      filterContains: '👉', requireURL: false, resolveRedirects: false },
  { handle: 'GithubProjects', filterContains: '',   requireURL: false, resolveRedirects: true  },
  { handle: 'tom_doerr',      filterContains: '',   requireURL: true,  resolveRedirects: false  },
];

const NITTER_INSTANCES = [
  'https://nitter.poast.org',
  'https://nitter.net',
  'https://nitter.privacydev.net',
  'https://nitter.unixfox.eu',
];

const HISTORY_MAX = 5;
const URL_RE = /https?:\/\/[^\s"<>\]]+/g;
const HREF_RE = /href="(https?:\/\/[^"]+)"/g;

// ── Init ──────────────────────────────────────────────────────────────────────

chrome.runtime.onInstalled.addListener(() => {
  chrome.alarms.create('poll', { periodInMinutes: POLL_INTERVAL_HOURS * 60 });
  pollAll();
});

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === 'poll') pollAll();
});

chrome.runtime.onMessage.addListener((msg, _sender, reply) => {
  if (msg.action === 'pollNow') pollAll().then(() => reply({ ok: true }));
  if (msg.action === 'getState') getState().then(reply);
  if (msg.action === 'skip')    doReview(msg.handle, msg.id, 'skipped').then(reply);
  if (msg.action === 'approve') doReview(msg.handle, msg.id, 'approved').then(reply);
  if (msg.action === 'addURL')  addURL(msg.url).then(reply);
  if (msg.action === 'removeURL') removeURL(msg.idx).then(reply);
  if (msg.action === 'clearSelection') clearSelection().then(reply);
  return true; // keep message channel open for async reply
});

// ── Polling ───────────────────────────────────────────────────────────────────

async function pollAll() {
  const state = await getState();
  let changed = false;

  for (const acct of ACCOUNTS) {
    try {
      const items = await fetchRSS(acct.handle);
      const queue = state.accounts[acct.handle] || { pending: [], history: [] };
      const knownIDs = new Set([
        ...queue.pending.map(t => t.id),
        ...queue.history.map(t => t.id),
      ]);

      for (const item of items) {
        if (knownIDs.has(item.id)) continue;

        const text = item.title + ' ' + item.desc;

        if (acct.filterContains && !text.includes(acct.filterContains)) continue;

        const urls = extractURLs(text, acct.handle);
        if (acct.requireURL && urls.length === 0) continue;

        let resolved = [];
        if (acct.resolveRedirects && urls.length > 0) {
          resolved = await Promise.all(urls.map(resolveRedirect));
          resolved = [...new Set(resolved.filter(u => u !== urls[resolved.indexOf(u)] || true))];
        }

        queue.pending.push({
          id: item.id,
          account: acct.handle,
          tweetURL: item.tweetURL,
          extractedURLs: urls,
          resolvedURLs: acct.resolveRedirects ? resolved : [],
          rawText: item.title,
          discoveredAt: item.pubDate,
          status: 'pending',
        });

        knownIDs.add(item.id);
        changed = true;
      }

      state.accounts[acct.handle] = queue;
    } catch (e) {
      console.warn(`[bm-scout] @${acct.handle}: ${e.message}`);
    }
  }

  state.lastPoll = Date.now();
  if (changed) await setState(state);
  else await chrome.storage.local.set({ lastPoll: state.lastPoll });

  updateBadge(state);
}

// ── Review (skip / approve) ───────────────────────────────────────────────────

async function doReview(handle, id, status) {
  const state = await getState();
  const queue = state.accounts[handle];
  if (!queue) return;

  const idx = queue.pending.findIndex(t => t.id === id);
  if (idx === -1) return;

  const [tweet] = queue.pending.splice(idx, 1);
  tweet.status = status;

  queue.history.unshift(tweet);
  if (queue.history.length > HISTORY_MAX) queue.history.pop();

  await setState(state);
  updateBadge(state);
}

// ── Selection ─────────────────────────────────────────────────────────────────

async function addURL(url) {
  const state = await getState();
  if (!state.selection.includes(url)) state.selection.push(url);
  await setState(state);
}

async function removeURL(idx) {
  const state = await getState();
  state.selection.splice(idx, 1);
  await setState(state);
}

async function clearSelection() {
  const state = await getState();
  state.selection = [];
  await setState(state);
}

// ── Storage ───────────────────────────────────────────────────────────────────

async function getState() {
  const data = await chrome.storage.local.get(null);
  const accounts = {};
  for (const a of ACCOUNTS) {
    accounts[a.handle] = data[`acct_${a.handle}`] || { pending: [], history: [] };
  }
  return {
    accounts,
    selection: data.selection || [],
    lastPoll: data.lastPoll || 0,
  };
}

async function setState(state) {
  const obj = { selection: state.selection, lastPoll: state.lastPoll };
  for (const [handle, queue] of Object.entries(state.accounts)) {
    obj[`acct_${handle}`] = queue;
  }
  await chrome.storage.local.set(obj);
}

// ── Badge ─────────────────────────────────────────────────────────────────────

function updateBadge(state) {
  const count = Object.values(state.accounts).reduce((n, q) => n + q.pending.length, 0);
  chrome.action.setBadgeText({ text: count > 0 ? String(count) : '' });
  chrome.action.setBadgeBackgroundColor({ color: '#3d7ae8' });
}

// ── RSS fetch + parse ─────────────────────────────────────────────────────────

async function fetchRSS(handle) {
  let lastErr;
  for (const instance of NITTER_INSTANCES) {
    try {
      const res = await fetch(`${instance}/${handle}/rss`, {
        signal: AbortSignal.timeout(10_000),
        headers: { 'Accept': 'application/rss+xml,application/xml,text/xml' },
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const xml = await res.text();
      return parseRSS(xml, instance);
    } catch (e) {
      lastErr = e;
    }
  }
  throw new Error(`All nitter instances failed: ${lastErr?.message}`);
}

function parseRSS(xml, nitterBase) {
  const items = [];
  const itemRe = /<item>([\s\S]*?)<\/item>/g;
  let m;
  while ((m = itemRe.exec(xml)) !== null) {
    const raw = m[1];
    const link   = extractXmlTag(raw, 'link');
    const title  = stripHTML(extractXmlTag(raw, 'title'));
    const desc   = extractXmlTag(raw, 'description');
    const pubDate = extractXmlTag(raw, 'pubDate');
    const id = extractTweetID(link);
    if (!id) continue;
    items.push({
      id,
      title,
      desc,
      pubDate,
      tweetURL: tweetXURL(link),
    });
  }
  return items;
}

function extractXmlTag(xml, tag) {
  const re = new RegExp(`<${tag}[^>]*>(?:<!\\[CDATA\\[)?([\\s\\S]*?)(?:\\]\\]>)?<\\/${tag}>`, 'i');
  const m = re.exec(xml);
  return m ? m[1].trim() : '';
}

function stripHTML(html) {
  return html.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
}

function extractTweetID(url) {
  const m = url.match(/\/status\/(\d+)/);
  return m ? m[1] : '';
}

function tweetXURL(nitterURL) {
  return nitterURL
    .replace(/^https?:\/\/[^/]+/, 'https://x.com')
    .replace(/#m$/, '');
}

// ── URL extraction ────────────────────────────────────────────────────────────

function extractURLs(text, handle) {
  const seen = new Set();
  const out = [];
  const add = (url) => {
    url = url.replace(/[.,;!)\]]+$/, '');
    if (!url.startsWith('http')) return;
    // Skip nitter self-links and t.co
    if (/nitter\.|t\.co/.test(url)) return;
    // Skip x.com / twitter.com links (those are tweet links, not tools)
    if (/^https?:\/\/(x\.com|twitter\.com)/.test(url)) return;
    const key = url.toLowerCase();
    if (seen.has(key)) return;
    seen.add(key);
    out.push(url);
  };
  let m;
  while ((m = HREF_RE.exec(text)) !== null) add(m[1]);
  HREF_RE.lastIndex = 0;
  while ((m = URL_RE.exec(text)) !== null) add(m[0]);
  URL_RE.lastIndex = 0;
  return out;
}

async function resolveRedirect(url) {
  try {
    const res = await fetch(url, {
      method: 'HEAD',
      redirect: 'follow',
      signal: AbortSignal.timeout(5_000),
    });
    return res.url || url;
  } catch {
    return url;
  }
}
