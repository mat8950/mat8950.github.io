// popup.js

let state       = null;
let activeHandle = null;
let currentTweet = null;
let isDragging   = false;
let dragStartX   = 0;

const $ = id => document.getElementById(id);

const card      = $('card');
const cardEmpty = $('card-empty');
const swipeRow  = $('swipe-row');

// Inject drag-feedback stamps into the card element
card.insertAdjacentHTML('afterbegin',
  '<div class="stamp add">ADD</div><div class="stamp nope">NOPE</div>'
);
const stampAdd  = card.querySelector('.stamp.add');
const stampNope = card.querySelector('.stamp.nope');

// ── Messaging ─────────────────────────────────────────────────────────────────

function sendMsg(msg) {
  return new Promise(resolve => chrome.runtime.sendMessage(msg, resolve));
}

// ── Boot ──────────────────────────────────────────────────────────────────────

async function load() {
  state = await sendMsg({ action: 'getState' });

  const handles = Object.keys(state.accounts);
  activeHandle  = handles.find(h => state.accounts[h].pending.length > 0) || handles[0];

  renderTabs();
  renderCard();
  renderHistory();
  renderSelection();
  renderLastPoll();
}

// ── Tabs ──────────────────────────────────────────────────────────────────────

function renderTabs() {
  const container = $('account-tabs');
  container.innerHTML = '';
  for (const handle of Object.keys(state.accounts)) {
    const count = state.accounts[handle].pending.length;
    const btn   = document.createElement('button');
    btn.className = 'tab' + (handle === activeHandle ? ' active' : '');
    btn.innerHTML = `@${handle}<span class="badge">${count || ''}</span>`;
    btn.onclick = () => {
      activeHandle = handle;
      renderTabs();
      renderCard();
      renderHistory();
    };
    container.appendChild(btn);
  }
}

// ── Card ──────────────────────────────────────────────────────────────────────

function renderCard() {
  const queue = state.accounts[activeHandle];
  currentTweet = queue?.pending[0] || null;

  if (!currentTweet) {
    card.style.display     = 'none';
    cardEmpty.style.display = '';
    swipeRow.style.display  = 'none';
    return;
  }

  card.style.display      = '';
  cardEmpty.style.display = 'none';
  swipeRow.style.display  = '';

  // Reset animation state
  card.className         = 'card';
  card.style.transform   = '';
  card.style.opacity     = '';
  stampAdd.style.opacity  = '0';
  stampNope.style.opacity = '0';

  $('card-account').textContent = `@${currentTweet.account}`;
  $('card-text').textContent    = currentTweet.rawText || '';
  $('card-ts').textContent      = currentTweet.discoveredAt
    ? new Date(currentTweet.discoveredAt).toLocaleString('fr-FR')
    : '';

  const urlsDiv = $('card-urls');
  urlsDiv.innerHTML = '';
  const resolved = currentTweet.resolvedURLs?.length > 0;
  const urls     = resolved ? currentTweet.resolvedURLs : (currentTweet.extractedURLs || []);
  for (const url of urls) {
    const pill = document.createElement('span');
    pill.className = 'url-pill' + (resolved ? ' resolved' : '');
    pill.textContent = url.replace(/^https?:\/\//, '');
    pill.title  = url;
    pill.onclick = e => { e.stopPropagation(); chrome.tabs.create({ url }); };
    urlsDiv.appendChild(pill);
  }

  $('btn-open').onclick = () => chrome.tabs.create({ url: currentTweet.tweetURL });
}

// ── Swipe ─────────────────────────────────────────────────────────────────────

async function swipe(dir) {
  if (!currentTweet) return;
  if (card.classList.contains('fly-left') || card.classList.contains('fly-right')) return;

  card.classList.add(dir === 'left' ? 'fly-left' : 'fly-right');

  if (dir === 'right') {
    await sendMsg({ action: 'approve', handle: activeHandle, id: currentTweet.id });
    const urls = currentTweet.resolvedURLs?.length
      ? currentTweet.resolvedURLs
      : (currentTweet.extractedURLs || []);
    for (const url of urls) await sendMsg({ action: 'addURL', url });
  } else {
    await sendMsg({ action: 'skip', handle: activeHandle, id: currentTweet.id });
  }

  setTimeout(async () => {
    state = await sendMsg({ action: 'getState' });
    renderTabs();
    renderCard();
    renderHistory();
    renderSelection();
  }, 380);
}

// ── Drag ──────────────────────────────────────────────────────────────────────

card.addEventListener('mousedown', e => {
  if (e.button !== 0) return;
  if (e.target.closest('.url-pill, #btn-open')) return;
  isDragging  = true;
  dragStartX  = e.clientX;
  card.classList.add('dragging');
});

document.addEventListener('mousemove', e => {
  if (!isDragging) return;
  const dx  = e.clientX - dragStartX;
  const rot = dx * 0.08;
  card.style.transform = `translateX(${dx}px) rotate(${rot}deg)`;
  const ratio = Math.min(Math.abs(dx) / 80, 1);
  if (dx > 0) {
    stampAdd.style.opacity  = ratio;
    stampNope.style.opacity = '0';
  } else {
    stampNope.style.opacity = ratio;
    stampAdd.style.opacity  = '0';
  }
});

document.addEventListener('mouseup', e => {
  if (!isDragging) return;
  isDragging = false;
  card.classList.remove('dragging');
  const dx = e.clientX - dragStartX;
  if (Math.abs(dx) > 80) {
    swipe(dx > 0 ? 'right' : 'left');
  } else {
    card.style.transform    = '';
    stampAdd.style.opacity  = '0';
    stampNope.style.opacity = '0';
  }
});

// ── Keyboard ──────────────────────────────────────────────────────────────────

document.addEventListener('keydown', e => {
  if (e.target.tagName === 'INPUT') return;
  switch (e.key) {
    case 'ArrowLeft':  swipe('left');  break;
    case 'ArrowRight': swipe('right'); break;
    case 'o': case 'O':
      if (currentTweet) chrome.tabs.create({ url: currentTweet.tweetURL });
      break;
  }
});

// ── Swipe buttons ─────────────────────────────────────────────────────────────

$('btn-skip').onclick    = () => swipe('left');
$('btn-approve').onclick = () => swipe('right');

$('btn-refresh').onclick = async () => {
  await sendMsg({ action: 'pollNow' });
  state = await sendMsg({ action: 'getState' });
  renderTabs();
  renderCard();
  renderHistory();
  renderSelection();
  renderLastPoll();
};

// ── History strip ─────────────────────────────────────────────────────────────

function renderHistory() {
  const strip = $('history-strip');
  strip.innerHTML = '';
  const hist = state.accounts[activeHandle]?.history || [];
  for (const tweet of hist) {
    const el = document.createElement('div');
    el.className = `hist-item ${tweet.status}`;
    el.title     = (tweet.rawText || '').slice(0, 80) || tweet.tweetURL;
    el.textContent = tweet.status === 'approved' ? '✓' : '✕';
    el.onclick = () => chrome.tabs.create({ url: tweet.tweetURL });
    strip.appendChild(el);
  }
}

// ── Selection panel ───────────────────────────────────────────────────────────

function renderSelection() {
  const list = $('sel-list');
  list.innerHTML = '';
  $('sel-count').textContent = state.selection.length;

  state.selection.forEach((url, idx) => {
    const item  = document.createElement('div');
    item.className = 'sel-item';

    const label = document.createElement('span');
    label.textContent = url.replace(/^https?:\/\//, '');
    label.title = url;

    const btn = document.createElement('button');
    btn.textContent = '×';
    btn.onclick = async () => {
      await sendMsg({ action: 'removeURL', idx });
      state = await sendMsg({ action: 'getState' });
      renderSelection();
    };

    item.appendChild(label);
    item.appendChild(btn);
    list.appendChild(item);
  });
}

$('btn-export').onclick = () => {
  if (!state.selection.length) return;
  navigator.clipboard.writeText(state.selection.join('\n'));
  const btn = $('btn-export');
  btn.textContent = 'Copié ✓';
  setTimeout(() => { btn.textContent = 'Copier'; }, 1500);
};

$('btn-clear-sel').onclick = async () => {
  await sendMsg({ action: 'clearSelection' });
  state = await sendMsg({ action: 'getState' });
  renderSelection();
};

const urlInput = $('url-input');
async function addManualURL() {
  const url = urlInput.value.trim();
  if (!url.startsWith('http')) return;
  await sendMsg({ action: 'addURL', url });
  state = await sendMsg({ action: 'getState' });
  urlInput.value = '';
  renderSelection();
}
$('btn-add-url').onclick = addManualURL;
urlInput.addEventListener('keydown', e => { if (e.key === 'Enter') addManualURL(); });

// ── Last poll ─────────────────────────────────────────────────────────────────

function renderLastPoll() {
  if (!state.lastPoll) { $('last-poll').textContent = ''; return; }
  const mins = Math.floor((Date.now() - state.lastPoll) / 60_000);
  $('last-poll').textContent = mins < 60 ? `${mins}m` : `${Math.floor(mins / 60)}h`;
}

// ── Go ────────────────────────────────────────────────────────────────────────

load();
