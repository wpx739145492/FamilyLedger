/* 添丁记 - 核心：状态、认证、导航、首页录入与洞察 */
window.FL = (function () {
  'use strict';

  var LS_TOKEN = 'fl_tk', LS_MEMBER = 'fl_me', LS_DATE = 'fl_date', LS_QUEUE = 'fl_q', LS_CAT = 'fl_cat';

  var CATS = [
    { n: '奶粉', u: '罐' }, { n: '尿不湿', u: '包' }, { n: '医疗', u: '' }, { n: '服饰', u: '件' },
    { n: '辅食', u: '盒' }, { n: '玩具', u: '件' }, { n: '早教', u: '' }, { n: '出行', u: '' },
    { n: '洗护', u: '瓶' }, { n: '纸巾湿巾', u: '包' }, { n: '摄影纪念', u: '' }, { n: '其他', u: '' }
  ];
  var QUICK = [200, 400, 500, 600, 800, 1000, 2000];

  var S = {
    token: localStorage.getItem(LS_TOKEN) || '',
    me: null, page: 'home', mode: 'gift', dir: 'out',
    person: null, date: localStorage.getItem(LS_DATE) || todayStr(),
    cat: localStorage.getItem(LS_CAT) || '奶粉', queue: loadQueue(), profile: {},
    babies: [], babyID: Number(localStorage.getItem('fl_baby_id')) || 1
  };

  var VIEWS = {};

  function $(id) { return document.getElementById(id); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function todayStr() { var d = new Date(); return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()); }
  function yesterday() { var d = new Date(Date.now() - 864e5); return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()); }
  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  function fmt(c) {
    var neg = c < 0, v = Math.abs(c);
    var y = Math.floor(v / 100), r = v % 100;
    var s = String(y).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    return (neg ? '-' : '') + s + (r ? '.' + (r < 10 ? '0' + r : r) : '');
  }
  function trimNum(s) { return String(s || '').replace(/[,¥￥\s]/g, ''); }
  function dayLabel(d) {
    if (d === todayStr()) return '今天';
    if (d === yesterday()) return '昨天';
    var p = d.split('-');
    return parseInt(p[1], 10) + '/' + parseInt(p[2], 10);
  }
  function diffDays(a, b) { return Math.round((new Date(b) - new Date(a)) / 864e5); }
  function ageText(birthday, on) {
    if (!birthday) return '';
    var d = diffDays(birthday, on || todayStr());
    if (d < 0) return '';
    if (d < 60) return '出生第 ' + d + ' 天';
    var m = Math.floor(d / 30.44), dd = Math.round(d - m * 30.44);
    if (m < 12) return m + ' 个月' + (dd > 0 ? dd + ' 天' : '');
    var y = Math.floor(m / 12), rm = m % 12;
    return y + ' 岁' + (rm ? rm + ' 个月' : '');
  }

  /* ---------- API ---------- */
  function loadQueue() { try { return JSON.parse(localStorage.getItem(LS_QUEUE) || '[]'); } catch (e) { return []; } }
  function saveQueue() { localStorage.setItem(LS_QUEUE, JSON.stringify(S.queue)); }
  function unwrap(r) {
    if (r.status === 401) { logout(); throw new Error('登录失效'); }
    var p = r.json().catch(function () { return {}; });
    return p.then(function (j) {
      if (!r.ok) { var e = new Error(j.error || ('请求失败 ' + r.status)); e.status = r.status; throw e; }
      return j;
    });
  }
  function api(path, opt) {
    opt = opt || {};
    var init = { method: opt.method || 'GET', headers: {}, cache: 'no-store' };
    if (S.token) init.headers['Authorization'] = 'Bearer ' + S.token;
    if (opt.body) { init.headers['Content-Type'] = 'application/json'; init.body = JSON.stringify(opt.body); }
    return fetch(path, init).then(unwrap);
  }
  function apiWrite(path, opt) {
    return api(path, opt).catch(function (err) {
      var st = err.status || 0;
      if (st >= 400 && st < 500) throw err;
      S.queue.push({ id: 'q' + Date.now() + Math.random().toString(36).slice(2, 6), path: path, opt: opt });
      saveQueue(); renderOffline();
      throw new Error('已离线暂存');
    });
  }
  function flushQueue() {
    if (!S.queue.length || !navigator.onLine) return;
    if (flushQueue._busy) return;
    flushQueue._busy = true;
    var q = S.queue.slice();
    (function next() {
      if (!q.length) { flushQueue._busy = false; return; }
      var it = q.shift();
      api(it.path, it.opt).then(function () {
        S.queue = S.queue.filter(function (x) { return x.id !== it.id; });
        saveQueue(); renderOffline(); next();
      }).catch(function (e) {
        if (e.status && e.status < 500) {
          S.queue = S.queue.filter(function (x) { return x.id !== it.id; }); saveQueue();
        }
        flushQueue._busy = false; renderOffline();
      });
    })();
  }
  function renderOffline() {
    var b = $('offlineBar'); if (!b) return;
    if (S.queue.length) { b.textContent = '离线中，' + S.queue.length + ' 条待同步'; b.classList.remove('hidden'); }
    else b.classList.add('hidden');
  }

  /* ---------- 登录 ---------- */
  function showLogin(msg) {
    $('main').classList.add('hidden'); $('login').classList.remove('hidden');
    $('loginSub').textContent = msg || '输入家庭口令'; $('memberGrid').innerHTML = '';
    $('pin').classList.remove('hidden'); $('btnLogin').classList.remove('hidden');
    setTimeout(function () { $('pin').focus(); }, 60);
  }
  function afterLogin(token, me) {
    S.token = token; S.me = me;
    localStorage.setItem(LS_TOKEN, token); localStorage.setItem(LS_MEMBER, JSON.stringify(me));
    $('login').classList.add('hidden'); $('main').classList.remove('hidden');
    boot();
  }
  function logout() {
    fetch('/api/logout', { method: 'POST', headers: S.token ? { Authorization: 'Bearer ' + S.token } : {} }).catch(function () { });
    localStorage.removeItem(LS_TOKEN); S.token = ''; S.me = null;
    showLogin('登录已失效，请重新输入');
  }
  function doLogin() {
    var pc = $('pin').value.trim();
    if (!pc) { $('loginErr').textContent = '请输入口令'; return; }
    $('loginErr').textContent = '…';
    api('/api/login', { method: 'POST', body: { passcode: pc } }).then(function (j) {
      $('loginErr').textContent = ''; $('loginSub').textContent = '你是谁？';
      $('pin').classList.add('hidden'); $('btnLogin').classList.add('hidden');
      var colors = ['#6b8cff', '#ef6f8a', '#2bb98a', '#f0ab3d'];
      $('memberGrid').innerHTML = (j.members || []).map(function (m, i) {
        return '<button class="member-btn" data-id="' + m.id + '"><div class="av" style="background:' + colors[i % 4] + '">' +
          esc(m.nickname.slice(0, 1)) + '</div><div class="nm">' + esc(m.nickname) + '</div></button>';
      }).join('') + '<button class="member-btn" data-id="new"><div class="av" style="background:#3b4353">+</div><div class="nm">新成员</div></button>';
      Array.prototype.forEach.call($('memberGrid').children, function (b) {
        b.onclick = function () {
          var id = b.getAttribute('data-id');
          if (id === 'new') {
            var nm = prompt('输入称呼，比如 姥姥');
            if (!nm) return;
            pickMember(pc, function (cb) {
              api('/api/members', { method: 'POST', body: { nickname: nm.trim() } }).then(function (r) { cb(r.id, nm.trim()); })
                .catch(function (e) { alert(e.message); });
            });
            return;
          }
          pickMember(pc, function (cb) { cb(parseInt(id, 10), ((j.members || []).filter(function (x) { return x.id === parseInt(id, 10); })[0] || {}).nickname); });
        };
      });
    }).catch(function (e) { $('loginErr').textContent = e.message || '口令不对'; $('pin').select(); });
  }
  function pickMember(pc, resolve) {
    resolve(function (id, nick) {
      api('/api/session', { method: 'POST', body: { passcode: pc, member_id: id } })
        .then(function (s) { afterLogin(s.token, { id: id, nickname: nick }); })
        .catch(function (e) { alert(e.message); });
    });
  }

  /* ---------- 弹层 / Toast ---------- */
  function openSheet(title, html, cb) {
    $('sheetBody').innerHTML = '<div class="sheet-bar"></div><div class="sheet-t">' + esc(title) +
      '<button class="sheet-close" id="sheetX">关闭</button></div>' + html;
    $('sheetRoot').classList.remove('hidden');
    $('sheetX').onclick = closeSheet;
    if (cb) cb();
  }
  function closeSheet() { $('sheetRoot').classList.add('hidden'); }
  var toastTimer = null;
  function toast(msg, undoFn) {
    $('toastRoot').innerHTML = '<div class="toast">' + esc(msg) +
      (undoFn ? ' <span class="undo" id="undoBtn">撤销</span>' : '') + '</div>';
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { $('toastRoot').innerHTML = ''; }, undoFn ? 5200 : 2200);
    if (undoFn) $('undoBtn').onclick = function () { undoFn(); $('toastRoot').innerHTML = ''; };
  }
  function confirmSheet(title, desc, okText, onOk) {
    openSheet(title, '<div style="color:var(--dim);font-size:14px;margin-bottom:16px">' + esc(desc) + '</div>' +
      '<div style="display:flex;gap:10px"><button class="btn" id="cfNo" style="flex:1">取消</button>' +
      '<button class="btn btn-primary" id="cfOk" style="flex:1">' + esc(okText) + '</button></div>');
    $('cfNo').onclick = closeSheet;
    $('cfOk').onclick = function () { closeSheet(); onOk(); };
  }

  /* ---------- 导航 ---------- */
  var TITLES = {
    home: ['添丁记', '记录每一份心意'],
    gift: ['人情往来', '谁给了多少，还欠谁'],
    grow: ['成长档案', '花销是长大的脚印'],
    me: ['我的', '设置与数据'],
    family: ['家庭协作', '事件、任务和采购清单']
  };
  function nav(p) {
    S.page = p;
    document.querySelectorAll('.page').forEach(function (el) { el.classList.add('hidden'); });
    var page = $('page-' + p); if (page) page.classList.remove('hidden');
    document.querySelectorAll('.tb').forEach(function (b) { b.classList.toggle('on', b.getAttribute('data-p') === p); });
    var t = TITLES[p] || TITLES.home;
    $('hdrTitle').textContent = t[0]; $('hdrSub').textContent = t[1];
    if (p === 'gift') VIEWS.gift && VIEWS.gift();
    if (p === 'grow') VIEWS.grow && VIEWS.grow();
    if (p === 'me') VIEWS.me && VIEWS.me();
    if (p === 'family') VIEWS.family && VIEWS.family();
    if (p === 'home') { loadInsights(); loadRecent(); }
    window.scrollTo(0, 0);
  }

  /* ---------- 首页：录入 ---------- */
  function setMode(m) {
    S.mode = m;
    document.querySelectorAll('.mode').forEach(function (b) { b.classList.toggle('on', b.getAttribute('data-mode') === m); });
    $('giftFields').classList.toggle('hidden', m !== 'gift');
    $('babyFields').classList.toggle('hidden', m !== 'baby');
    $('dirSwitch').classList.toggle('hidden', m !== 'gift');
    $('babyPickerWrap').classList.toggle('hidden', m !== 'baby' || S.babies.length < 2);
    if (m === 'baby') syncUnit();
  }
  function syncUnit() {
    var u = (CATS.filter(function (c) { return c.n === S.cat; })[0] || {}).u || '';
    $('unitLabel').textContent = u || '—';
  }
  function pickDate() {
    openSheet('这笔是哪天的',
      '<div class="field"><input id="dateIn" class="field-in" type="date" value="' + S.date + '"></div>' +
      '<div class="cat-grid"><button class="cat" data-d="' + todayStr() + '">今天</button>' +
      '<button class="cat" data-d="' + yesterday() + '">昨天</button></div>' +
      '<button class="save" id="dateOk">确 定</button>');
    document.querySelectorAll('.sheet .cat[data-d]').forEach(function (b) {
      b.onclick = function () { $('dateIn').value = b.getAttribute('data-d'); };
    });
    $('dateOk').onclick = function () {
      S.date = $('dateIn').value || S.date; localStorage.setItem(LS_DATE, S.date);
      closeSheet(); renderChips();
    };
  }
  function dateChipText() {
    if (S.date === todayStr()) return '今天';
    if (S.date === yesterday()) return '昨天';
    var p = S.date.split('-'); return parseInt(p[1], 10) + '月' + parseInt(p[2], 10) + '日';
  }
  function renderChips() {
    var last = S.person && S.person.last_cents ? S.person.last_cents / 100 : 0;
    var html = '<button class="chip" data-kind="date">' + esc(dateChipText()) + '</button>';
    html += QUICK.map(function (v) { return '<button class="chip" data-amt="' + v + '">' + v + '</button>'; }).join('');
    if (last) html += '<button class="chip last" data-amt="' + last + '">上次 ' + last + '</button>';
    $('chips').innerHTML = html;
    $('chips').querySelectorAll('.chip').forEach(function (c) {
      c.onclick = function () {
        if (c.getAttribute('data-kind') === 'date') { pickDate(); return; }
        $('amt').value = c.getAttribute('data-amt');
      };
    });
  }
  function renderCats() {
    $('catGrid').innerHTML = CATS.map(function (c) {
      return '<button class="cat' + (c.n === S.cat ? ' on' : '') + '" data-c="' + esc(c.n) + '">' + esc(c.n) + '</button>';
    }).join('');
    $('catGrid').querySelectorAll('.cat').forEach(function (b) {
      b.onclick = function () { S.cat = b.getAttribute('data-c'); localStorage.setItem(LS_CAT, S.cat); renderCats(); syncUnit(); };
    });
  }

  var sugTimer = null;
  function onWho() {
    var q = $('who').value.trim();
    if (S.person && S.person.name !== q) { S.person = null; $('personCard').innerHTML = ''; }
    clearTimeout(sugTimer);
    sugTimer = setTimeout(function () { searchWho(q); }, 180);
  }
  function searchWho(q) {
    if (!q) { $('sug').innerHTML = ''; return; }
    api('/api/persons?q=' + encodeURIComponent(q) + '&limit=10').then(function (j) {
      var list = j.persons || [];
      var html = list.map(function (p) {
        return '<div class="sug-item" data-id="' + p.id + '"><div class="sug-av">' + esc(p.name.slice(0, 1)) + '</div>' +
          '<div><div class="sug-nm">' + esc(p.name) + (p.relation ? ' <span class="sug-sub">' + esc(p.relation) + '</span>' : '') + '</div>' +
          '<div class="sug-sub">' + oweText(p) + '</div></div></div>';
      }).join('');
      if (!list.some(function (p) { return p.name === q; })) {
        html += '<div class="sug-item" data-new="1"><div class="sug-av" style="background:#3b4353">＋</div>' +
          '<div><div class="sug-nm">新建「' + esc(q) + '」</div></div></div>';
      }
      $('sug').innerHTML = html;
      $('sug').querySelectorAll('.sug-item').forEach(function (it) {
        it.onclick = function () {
          var id = it.getAttribute('data-id');
          if (id) selectPerson(list.filter(function (x) { return x.id === +id; })[0]);
          else api('/api/persons', { method: 'POST', body: { name: q } })
            .then(function (r) { selectPerson(r.person); }).catch(function (e) { toast(e.message); });
        };
      });
    }).catch(function () { });
  }
  function oweText(p) {
    if (!p.count) return '新联系人';
    var n = p.net_cents;
    if (n > 0) return '往来 ' + p.count + ' 次 · <span style="color:var(--warn)">你还欠 ' + fmt(n) + '</span>';
    if (n < 0) return '往来 ' + p.count + ' 次 · <span style="color:var(--out)">对方欠 ' + fmt(-n) + '</span>';
    return '往来 ' + p.count + ' 次 · 已持平';
  }
  function selectPerson(p) {
    S.person = p; $('who').value = p.name; $('sug').innerHTML = '';
    renderPersonCard(); renderChips(); $('amt').focus();
  }
  function renderPersonCard() {
    var p = S.person;
    if (!p) { $('personCard').innerHTML = ''; return; }
    var n = p.net_cents, cls = 'pc-net', txt;
    if (n > 0) txt = '你还欠 ' + fmt(n);
    else if (n < 0) { txt = '对方欠 ' + fmt(-n); cls += ' ok'; }
    else txt = '已持平';
    var last = p.last_cents ? '<div class="pc-row" style="color:#68748a">上次 ' +
      (p.last_dir === 'in' ? '收 ' : '送 ') + fmt(p.last_cents) +
      (p.last_event ? ' · ' + esc(p.last_event) : '') + (p.last_on ? '（' + esc(p.last_on) + '）' : '') + '</div>' : '';
    $('personCard').innerHTML = '<div class="person-card"><div class="pc-top"><span class="pc-nm">' + esc(p.name) + '</span>' +
      (p.relation ? '<span class="pc-rel">' + esc(p.relation) + '</span>' : '') + '</div>' +
      '<div class="pc-row"><span>收 ' + fmt(p.recv_cents) + '</span><span>送 ' + fmt(p.sent_cents) + '</span>' +
      '<span class="' + cls + '">' + txt + '</span></div>' + last + '</div>';
  }

  function save() {
    var amt = trimNum($('amt').value);
    if (!amt) { toast('先填金额'); $('amt').focus(); return; }
    $('btnSave').disabled = true;
    var done = false;
    var req;
    if (S.mode === 'gift') {
      var who = $('who').value.trim();
      if (!who) { $('btnSave').disabled = false; toast('填一下是谁'); $('who').focus(); return; }
      req = {
        direction: S.dir, amount: amt, person_name: who,
        event_name: $('evt').value.trim(), date: S.date
      };
      if (S.person) req.person_id = S.person.id;
    } else {
      req = {
        category: S.cat, amount: amt, date: S.date,
        baby_id: S.babyID,
        qty: parseFloat(trimNum($('qty').value)) || 0,
        merchant: $('mer').value.trim(), note: $('note').value.trim()
      };
    }
    apiWrite('/api/' + (S.mode === 'gift' ? 'gifts' : 'baby'), { method: 'POST', body: req }).then(function (j) {
      done = true; $('btnSave').disabled = false;
      var label = S.mode === 'gift'
        ? (S.dir === 'in' ? '收到 ' : '送给 ') + (S.person ? S.person.name : who) + ' ¥' + amt
        : S.cat + ' ¥' + amt;
      toast(label, function () {
        apiWrite('/api/' + (S.mode === 'gift' ? 'gifts' : 'baby') + '/' + j.id, { method: 'DELETE' }).then(function () {
          loadInsights(); loadRecent();
        });
      });
      $('amt').value = '';
      if (S.mode === 'baby') { $('qty').value = ''; $('note').value = ''; $('mer').value = ''; }
      if (S.person && j.person) S.person = j.person;
      renderPersonCard(); renderChips();
      loadInsights(); loadRecent();
    }).catch(function (e) {
      $('btnSave').disabled = false;
      if (e.message === '已离线暂存') { toast('已暂存，联网自动同步'); $('amt').value = ''; }
      else if (!done) toast(e.message || '保存失败');
    });
  }

  /* ---------- 首页：洞察 ---------- */
  function loadInsights() {
    api(babyQuery('/api/insights')).then(function (j) {
      S.ins = j.insights;
      renderInsights(S.ins);
      $('hdrSub').textContent = S.profile.baby_name && S.profile.baby_birthday
        ? (S.profile.baby_name + ' · ' + ageText(S.profile.baby_birthday))
        : '记录每一份心意';
    }).catch(function () { });
  }

  function renderInsights(v) {
    if (!v) { $('insightBox').innerHTML = ''; return; }
    var name = (S.profile && S.profile.baby_name) || '宝宝';
    var html = '<div class="hero"><div class="hero-lb">' + esc(name) + '本月花销</div>' +
      '<div class="hero-v">¥' + fmt(v.month_cents) + '</div><div class="hero-row">' +
      '<span>上月 ¥' + fmt(v.last_month_cents) + (v.last_month_cents > 0 ? ' <span class="delta ' +
      (v.month_delta >= 0 ? 'up' : 'down') + '">' + (v.month_delta >= 0 ? '↑' : '↓') +
      Math.abs(v.month_delta) + '%</span>' : '') + '</span>' +
      '<span>累计 ¥' + fmt(v.baby_total) + '</span>' +
      (v.project_year_cents ? '<span>按现在节奏一年约 ¥' + fmt(v.project_year_cents) + '</span>' : '') +
      '</div></div>';

    if ((v.alerts || []).length) {
      html += v.alerts.slice(0, 5).map(function (a) {
        return '<div class="alert ' + a.level + '"' + (a.person_id ? ' data-pid="' + a.person_id + '"' : '') + '>' +
          '<span class="dot"></span><div class="alert-b"><div class="alert-t">' + esc(a.title) + '</div>' +
          '<div class="alert-d">' + esc(a.desc) + '</div></div></div>';
      }).join('');
    }

    if ((v.consumables || []).length) {
      var soon = v.consumables.filter(function (c) { return c.days_left <= 30; });
      if (soon.length) {
        html += '<div class="card"><div class="card-h"><span class="card-t">补货提醒</span></div>';
        html += soon.map(function (c) {
          var pct = Math.max(3, Math.min(100, Math.round(c.days_left / 30 * 100)));
          var color = c.days_left <= 3 ? 'var(--in)' : (c.days_left <= 10 ? 'var(--warn)' : 'var(--out)');
          return '<div class="supply-row"><span class="supply-nm">' + esc(c.category) + '</span>' +
            '<span class="supply-bar"><span class="supply-fl" style="width:' + pct + '%;background:' + color + '"></span></span>' +
            '<span class="supply-v">约 ' + c.days_left + ' 天</span></div>';
        }).join('') + '</div>';
      }
    }
    $('insightBox').innerHTML = html;
    $('insightBox').querySelectorAll('.alert[data-pid]').forEach(function (el) {
      el.onclick = function () {
        nav('gift');
      };
    });
  }

  /* ---------- 首页：最近 ---------- */
  function loadRecent() {
    api(babyQuery('/api/timeline?limit=12')).then(function (j) {
      $('recent').innerHTML = timelineHTML(j.items || []);
      bindTimeline($('recent'));
    }).catch(function () { });
  }
  function timelineHTML(items) {
    if (!items.length) return '<div class="empty">还没有记录，从上面记第一笔开始</div>';
    var colors = { gift: 'var(--in)', baby: 'var(--out)', milestone: 'var(--accent)' };
    return '<div>' + items.map(function (it) {
      var badge = '';
      if (it.type === 'gift') badge = '<span class="tl-badge" style="background:var(--in-soft);color:var(--in)">' + esc(it.badge) + '</span>';
      else if (it.type === 'baby') badge = '<span class="tl-badge" style="background:var(--out-soft);color:var(--out)">' + esc(it.badge) + '</span>';
      else badge = '<span class="tl-badge" style="background:var(--accent-soft);color:var(--accent)">' + esc(it.badge) + '</span>';
      var amt = it.cents ? ' <span style="color:' + (it.dir === 'in' ? 'var(--in)' : 'var(--out)') + ';font-weight:600">' +
        (it.dir === 'in' ? '+' : '-') + fmt(it.cents) + '</span>' : '';
      var age = it.type === 'milestone' && S.profile.baby_birthday ? ' · <span class="ms-age">' + esc(ageText(S.profile.baby_birthday, it.date)) + '</span>' : '';
      return '<div class="tl"><div class="tl-rail"><div class="tl-dot" style="background:' + colors[it.type] + '"></div>' +
        '<div class="tl-line"></div></div><div class="tl-row"><div class="tl-t">' + badge + esc(it.title) + amt + '</div>' +
        '<div class="tl-s">' + esc(it.date) + age + (it.sub ? ' · ' + esc(it.sub) : '') + '</div></div></div>';
    }).join('') + '</div>';
  }
  function bindTimeline(root) { }

  /* ---------- 语音填写 ---------- */
  function voiceFill() {
    var Recognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!Recognition) { toast('当前浏览器不支持语音填写'); return; }
    var rec = new Recognition();
    rec.lang = 'zh-CN'; rec.interimResults = false; rec.maxAlternatives = 1;
    toast('请说：给王姨送六百，周岁宴');
    rec.onresult = function (e) {
      var text = (e.results[0][0].transcript || '').replace(/[，。！？]/g, ' ');
      var money = text.match(/([0-9]+(?:\.[0-9]+)?)\s*(元|块)?|([一二两三四五六七八九十百千万]+)\s*(元|块)/);
      var amount = money ? (money[1] || chineseMoney(money[3])) : '';
      if (amount) $('amt').value = amount;
      var dir = /收到|收了|拿到|进账/.test(text) ? 'in' : (/送|给了|随了|支出/.test(text) ? 'out' : S.dir);
      S.dir = dir;
      document.querySelectorAll('.dir-btn').forEach(function (b) { b.classList.toggle('on', b.getAttribute('data-dir') === dir); });
      var who = text.replace(/(收到|收了|拿到|进账|送给|送了|给了|随了|支出)/g, ' ').replace(/([0-9]+(?:\.[0-9]+)?|[一二两三四五六七八九十百千万]+)\s*(元|块)/g, ' ').trim();
      who = who.replace(/(今天|昨天|前天|周岁宴|满月酒|百日宴|生日礼|春节红包).*$/, '').trim();
      if (who && $('who')) { $('who').value = who; $('who').dispatchEvent(new Event('input')); }
      var evt = text.match(/(满月酒|百日宴|周岁宴|婚礼|乔迁|生日礼|春节红包)/);
      if (evt && $('evt')) $('evt').value = evt[1];
      toast('已填写语音内容，请核对后保存');
    };
    rec.onerror = function () { toast('没有听清，请再试一次'); };
    rec.start();
  }
  function chineseMoney(s) {
    var digits = { '零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9 };
    var units = { '十': 10, '百': 100, '千': 1000, '万': 10000 };
    var total = 0, section = 0, number = 0;
    String(s || '').split('').forEach(function (c) {
      if (digits[c] != null) number = digits[c];
      else if (units[c] >= 10) { section += (number || 1) * units[c]; number = 0; if (units[c] === 10000) { total += section; section = 0; } }
    });
    return String(total + section + number);
  }

  /* ---------- 启动 / 绑定 ---------- */
  function boot() {
    api('/api/profile').then(function (j) {
      S.profile = j.profile || {};
      if (!S.profile.baby_name) S.profile.baby_name = '宝宝';
      loadInsights();
    }).catch(function () { loadInsights(); });
    api('/api/babies').then(function (j) {
      S.babies = j.babies || [];
      // 优先用本机记住的宝宝；没记住（换手机 / iOS 清了缓存）时退回后端标记的当前宝宝。
      var stored = Number(localStorage.getItem('fl_baby_id')) || 0;
      var match = S.babies.filter(function (b) { return b.id === stored; })[0];
      var act = S.babies.filter(function (b) { return b.active; })[0];
      S.babyID = (match || act || S.babies[0] || { id: 1 }).id;
      localStorage.setItem('fl_baby_id', String(S.babyID));
      refreshBabyPickers();
    }).catch(function () { });
    renderChips(); renderCats(); setMode('gift'); nav('home'); renderOffline(); flushQueue();
    if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js').catch(function () { });
  }

  function babyQuery(path) {
    return path + (path.indexOf('?') >= 0 ? '&' : '?') + 'baby_id=' + encodeURIComponent(S.babyID || 1);
  }
  function babyPickerHTML(id) {
    return '<div class="field"><span class="field-lb">当前宝宝</span><select id="' + id + '" class="field-in baby-picker">' +
      S.babies.map(function (b) { return '<option value="' + b.id + '"' + (b.id === S.babyID ? ' selected' : '') + '>' + esc(b.name) + '</option>'; }).join('') + '</select></div>';
  }
  function refreshBabyPickers() {
    document.querySelectorAll('.baby-picker').forEach(function (select) {
      var keep = select.value;
      // 列出全部宝宝：只列 active 的话，切到二宝后大宝就从下拉里消失了，再也切不回去。
      select.innerHTML = S.babies.map(function (b) { return '<option value="' + b.id + '">' + esc(b.name) + '</option>'; }).join('');
      select.value = String(S.babyID || keep || 1);
      select.onchange = function () {
        S.babyID = Number(select.value) || 1;
        localStorage.setItem('fl_baby_id', String(S.babyID));
        api('/api/babies/' + S.babyID + '/activate', { method: 'POST' }).catch(function () { });
        document.querySelectorAll('.baby-picker').forEach(function (x) { x.value = String(S.babyID); });
        loadInsights(); loadRecent();
        if (S.page === 'grow' && VIEWS.grow) VIEWS.grow();
      };
    });
    $('babyPickerWrap').classList.toggle('hidden', S.mode !== 'baby' || S.babies.length < 2);
  }

  function wire() {
    $('btnLogin').onclick = doLogin;
    $('pin').addEventListener('keydown', function (e) { if (e.key === 'Enter') doLogin(); });
    $('sheetMask').onclick = function (e) { if (e.target === $('sheetMask')) closeSheet(); };
    document.querySelectorAll('.tb').forEach(function (b) {
      b.onclick = function () { nav(b.getAttribute('data-p')); };
    });
    document.querySelectorAll('.mode').forEach(function (b) {
      b.onclick = function () { setMode(b.getAttribute('data-mode')); };
    });
    document.querySelectorAll('.dir-btn').forEach(function (b) {
      b.onclick = function () {
        S.dir = b.getAttribute('data-dir');
        document.querySelectorAll('.dir-btn').forEach(function (x) { x.classList.toggle('on', x === b); });
        $('amt').focus();
      };
    });
    $('amt').addEventListener('keydown', function (e) { if (e.key === 'Enter') save(); });
    $('who').addEventListener('input', onWho);
    $('who').addEventListener('keydown', function (e) { if (e.key === 'Enter') save(); });
    $('btnSave').onclick = save;
    if ($('btnVoice')) $('btnVoice').onclick = voiceFill;
    $('btnExport').onclick = function () {
      openSheet('导出到电脑', '<div style="color:var(--dim);font-size:13px;margin-bottom:14px">导出 CSV 文件，Excel / WPS 直接打开，中文不会乱码。</div>' +
        '<button class="btn btn-primary" id="exG" style="width:100%;margin-bottom:10px;padding:13px">人情账本（红包往来）</button>' +
        '<button class="btn btn-primary" id="exB" style="width:100%;padding:13px">宝宝开销明细</button>');
      $('exG').onclick = function () { location.href = '/api/export.csv?book=gift'; closeSheet(); };
      $('exB').onclick = function () { location.href = '/api/export.csv?book=baby'; closeSheet(); };
    };
    $('lnkAll').onclick = function () { VIEWS.timelineSheet && VIEWS.timelineSheet(); };
    $('lnkAddMs').onclick = function () { VIEWS.addMilestone && VIEWS.addMilestone(); };
    window.addEventListener('online', function () { renderOffline(); flushQueue(); });
    setInterval(function () { if (S.queue.length) flushQueue(); }, 30000);
  }

  function init() {
    wire();
    if (S.token) {
      try { S.me = JSON.parse(localStorage.getItem(LS_MEMBER) || 'null'); } catch (e) { }
      api('/api/gifts/summary').then(function () {
        $('login').classList.add('hidden'); $('main').classList.remove('hidden'); boot();
      }).catch(function () { showLogin(); });
    } else showLogin();
  }

  return {
    S: S, api: api, apiWrite: apiWrite, flushQueue: flushQueue, fmt: fmt, esc: esc, toast: toast,
    openSheet: openSheet, closeSheet: closeSheet, confirmSheet: confirmSheet, nav: nav, init: init,
    dayLabel: dayLabel, ageText: ageText, diffDays: diffDays, todayStr: todayStr,
    loadInsights: loadInsights, loadRecent: loadRecent, timelineHTML: timelineHTML,
    babyQuery: babyQuery, babyPickerHTML: babyPickerHTML, refreshBabyPickers: refreshBabyPickers,
    CATS: CATS, logout: logout, registerViews: function (v) { Object.assign(VIEWS, v); }, $: $
  };
})();
FL.init();
