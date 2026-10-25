/* 添丁记 - 子页面：往来 / 成长 / 我的 */
(function () {
  'use strict';
  var F = window.FL, S = F.S, $ = F.$, esc = F.esc, fmt = F.fmt, api = F.api, toast = F.toast;

  var PALETTE = ['#6b8cff', '#ef5f4c', '#2bb98a', '#f0ab3d', '#a97bff', '#3fb6d9', '#ef7ba9', '#8fd94a', '#c0a06b', '#5f7690'];

  /* ============ 工具：环形图 ============ */
  function drawRing(cv, data, total) {
    var dpr = window.devicePixelRatio || 1;
    var size = 108;
    cv.width = size * dpr; cv.height = size * dpr;
    cv.style.width = size + 'px'; cv.style.height = size + 'px';
    var g = null;
    try { g = cv.getContext('2d'); } catch (e) { g = null; }
    if (!g) return; // 环境不支持 canvas 时静默跳过（图表是增强，不是主干）
    g.scale(dpr, dpr);
    var cx = size / 2, cy = size / 2, r = size / 2 - 8, lw = 15;
    g.clearRect(0, 0, size, size);
    if (!total) {
      g.strokeStyle = '#242c3a'; g.lineWidth = lw;
      g.beginPath(); g.arc(cx, cy, r - lw / 2, 0, Math.PI * 2); g.stroke(); return;
    }
    var start = -Math.PI / 2;
    data.forEach(function (d) {
      var ang = d.value / total * Math.PI * 2;
      g.strokeStyle = d.color; g.lineWidth = lw; g.lineCap = 'butt';
      g.beginPath(); g.arc(cx, cy, r - lw / 2, start, start + ang); g.stroke();
      start += ang;
      if (start > Math.PI * 2) start -= Math.PI * 2;
    });
    // 中心文字
    g.fillStyle = '#e9edf5'; g.font = '600 15px -apple-system,sans-serif'; g.textAlign = 'center';
    g.fillText('¥' + fmt(total), cx, cy + 1);
    g.fillStyle = '#68748a'; g.font = '11px -apple-system,sans-serif';
    g.fillText('总计', cx, cy + 16);
  }

  /* ============ 日历热力图 ============ */
  function ymd(d) { return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'); }

  function heatHTML(days) {
    var map = {}, max = 1, total = 0;
    days.forEach(function (d) {
      map[d.date] = (map[d.date] || 0) + d.amount_cents;
      if (map[d.date] > max) max = map[d.date];
      total += d.amount_cents;
    });
    var today = new Date();
    var wd = today.getDay(); if (wd === 0) wd = 7;
    var monday = new Date(today.getTime() - (wd - 1) * 864e5);
    var rows = '';
    for (var w = 4; w >= 0; w--) {
      var cells = '';
      for (var d = 0; d < 7; d++) {
        var dt = new Date(monday.getTime() + (d - w * 7) * 864e5);
        var key = ymd(dt);
        var amt = map[key] || 0;
        var lvl = amt ? Math.max(1, Math.min(4, Math.ceil(amt / max * 4))) : 0;
        var future = dt > today;
        cells += '<div class="heat-c l' + lvl + (future ? ' future' : '') + '" title="' + key + ' ¥' + fmt(amt) + '">' +
          dt.getDate() + '</div>';
      }
      rows += '<div class="heat-row">' + cells + '</div>';
    }
    return '<div class="card-h"><span class="card-t">哪天花了钱</span>' +
      '<span class="stat-x">近 5 周 · 共 ¥' + fmt(total) + '</span></div>' +
      '<div class="heat">' + rows + '</div>' +
      '<div style="display:flex;align-items:center;gap:6px;margin-top:10px;font-size:11px;color:var(--dim2)">' +
      '<span>少</span><span class="heat-c l1" style="width:14px;height:14px"></span>' +
      '<span class="heat-c l2" style="width:14px;height:14px"></span>' +
      '<span class="heat-c l3" style="width:14px;height:14px"></span>' +
      '<span class="heat-c l4" style="width:14px;height:14px"></span><span>多</span></div>';
  }

  /* ============ 往来 ============ */
  var RANGES = [['week', '本周'], ['month', '本月'], ['quarter', '本季'], ['year', '本年'], ['all', '全部']];
  var curRange = 'month';
  var curFilter = {};

  function rangeHTML() {
    return '<div class="mode-switch" id="rangeSwitch">' + RANGES.map(function (r) {
      return '<button class="mode' + (r[0] === curRange ? ' on' : '') + '" data-r="' + r[0] + '">' + r[1] + '</button>';
    }).join('') + '</div>';
  }
  function deltaHTML(cur, prev, unit) {
    if (!prev) return '';
    var d = Math.round((cur - prev) / prev * 100);
    if (d === 0) return '<span style="color:var(--dim2)">与上次持平</span>';
    return '<span class="delta ' + (d > 0 ? 'up' : 'down') + '">' + (d > 0 ? '↑' : '↓') + Math.abs(d) + '%</span>' +
      '<span style="color:var(--dim2)"> 上次 ¥' + fmt(prev) + '</span>';
  }

  function gift() {
    var box = $('giftOverview'), list = $('giftList'), evBox = $('giftByEvent');
    box.innerHTML = '<div class="card"><div class="skel" style="height:80px"></div></div>';

    // 区间统计
    api('/api/stats?range=' + curRange).then(function (j) {
      var s = j.stats;
      var html = rangeHTML();

      html += '<div class="stat-grid">' +
        '<div class="stat"><div class="stat-lb">收到礼金</div><div class="stat-v in">¥' + fmt(s.gift_recv) + '</div>' +
        '<div class="stat-x">' + deltaHTML(s.gift_recv, s.prev_gift_recv) + '</div></div>' +
        '<div class="stat"><div class="stat-lb">送出礼金</div><div class="stat-v out">¥' + fmt(s.gift_sent) + '</div>' +
        '<div class="stat-x">' + deltaHTML(s.gift_sent, s.prev_gift_sent) + '</div></div>' +
        '<div class="stat"><div class="stat-lb">净差额</div><div class="stat-v ' + (s.gift_net > 0 ? 'warn' : 'acc') + '">¥' + fmt(s.gift_net) + '</div>' +
        '<div class="stat-x">' + s.gift_count + ' 笔 · ' + s.active_persons + ' 人走动</div></div>' +
        '<div class="stat"><div class="stat-lb">宝宝开销</div><div class="stat-v">¥' + fmt(s.baby_total) + '</div>' +
        '<div class="stat-x">' + deltaHTML(s.baby_total, s.prev_baby_total) + '</div></div>' +
        '</div>';

      if (s.new_persons > 0) {
        html += '<div style="font-size:12px;color:var(--dim2);padding:2px 4px 10px">本期新增 ' + s.new_persons + ' 位往来对象</div>';
      }

      if ((s.top_persons || []).length) {
        var mx = s.top_persons[0].amount_cents || 1;
        html += '<div class="card" style="margin-top:12px"><div class="card-h"><span class="card-t">往来最多</span></div>' +
          s.top_persons.map(function (c) {
            return '<div class="bar-row"><span class="bar-lb">' + esc(c.category) + '</span>' +
              '<span class="bar-bg"><span class="bar-fl" style="width:' + Math.round(c.amount_cents / mx * 100) + '%;background:var(--in)"></span></span>' +
              '<span class="bar-v">¥' + fmt(c.amount_cents) + '</span></div>';
          }).join('') + '</div>';
      }

      // 操作区
      html += '<div style="display:flex;gap:8px;margin:12px 0">' +
        '<button class="btn btn-primary" id="btnBatch" style="flex:1;padding:12px">批量录礼单</button>' +
        '<button class="btn" id="btnFilter" style="flex:1;padding:12px">筛选' + filterBadge() + '</button>' +
        '</div>';

      box.innerHTML = html;
      box.querySelectorAll('#rangeSwitch .mode').forEach(function (b) {
        b.onclick = function () { curRange = b.getAttribute('data-r'); gift(); };
      });
      $('btnBatch').onclick = batchSheet;
      $('btnFilter').onclick = filterSheet;

      return api('/api/owes').then(function (o) {
        var owes = o.persons || [];
        var extra = '';
        if (owes.length) {
          var scored = owes.map(function (p) {
            // 紧迫度按「最后一次收到对方礼金」起算
            var base = p.last_in_on || p.last_on;
            var days = Math.max(0, Math.round((Date.now() - new Date(base)) / 864e5));
            return { p: p, days: days, score: p.net_cents * Math.log(1 + days / 30) };
          }).sort(function (a, b) { return b.score - a.score; });
          extra += '<div class="card"><div class="card-h"><span class="card-t">待回礼清单</span>' +
            '<span class="stat-x">越拖越失礼</span></div>';
          extra += scored.map(function (x) {
            var p = x.p;
            var lv = x.days >= 365 ? 'urgent' : (x.days >= 120 ? 'warn' : 'info');
            var tip = x.days >= 365 ? '已拖一年多' : (x.days >= 120 ? '有些久了' : (x.days >= 60 ? '可以安排了' : '还不急'));
            return '<div class="alert ' + lv + '" data-pid="' + p.id + '"><span class="dot"></span>' +
              '<div class="alert-b"><div class="alert-t">' + esc(p.name) + ' · 还欠 ' + fmt(p.net_cents) + '</div>' +
              '<div class="alert-d">' + tip + ' · 距今 ' + x.days + ' 天 · 累计收 ' +
              fmt(p.recv_cents) + ' / 送 ' + fmt(p.sent_cents) + '</div></div></div>';
          }).join('') + '</div>';
        } else {
          extra += '<div class="card"><div class="card-t">待回礼清单</div><div class="empty">没有欠人情的人，账目很干净</div></div>';
        }
        box.insertAdjacentHTML('beforeend', extra);
        box.querySelectorAll('.alert[data-pid]').forEach(function (el) {
          el.onclick = function () { personSheet(+el.getAttribute('data-pid')); };
        });
      });
    }).catch(function (e) { box.innerHTML = '<div class="card"><div class="empty">' + esc(e.message) + '</div></div>'; });

    // 全部往来（带筛选）
    list.innerHTML = '<div class="skel" style="height:60px;margin-bottom:7px"></div>';
    loadGiftList();

    // 按事由
    evBox.innerHTML = '';
    api('/api/events').then(function (j) {
      var arr = (j.events || []).filter(function (e) { return e.count > 0; });
      if (!arr.length) return;
      var max = arr[0].count || 1;
      evBox.innerHTML = '<div class="card">' + arr.slice(0, 8).map(function (e) {
        return '<div class="bar-row"><span class="bar-lb">' + esc(e.name) + '</span>' +
          '<span class="bar-bg"><span class="bar-fl" style="width:' + Math.round(e.count / max * 100) + '%"></span></span>' +
          '<span class="bar-v">' + e.count + ' 笔</span></div>';
      }).join('') + '</div>';
    }).catch(function () { });
  }

  function filterBadge() { return Object.keys(curFilter).length ? ' · ' + Object.keys(curFilter).length : ''; }
  function filterQS() {
    var p = [];
    for (var k in curFilter) if (curFilter[k]) p.push(k + '=' + encodeURIComponent(curFilter[k]));
    return p.length ? '&' + p.join('&') : '';
  }
  function loadGiftList() {
    var list = $('giftList');
    api('/api/gifts?limit=200' + filterQS()).then(function (j) {
      var arr = j.records || [];
      if (!arr.length) { list.innerHTML = '<div class="empty">没有符合条件的记录</div>'; return; }
      list.innerHTML = arr.map(function (g) {
        return '<div class="rec" data-gid="' + g.id + '"><span class="rec-dt">' + esc(F.dayLabel(g.occurred_on)) + '</span>' +
          '<div class="rec-mid"><div class="rec-nm">' + esc(g.person_name) +
          (g.event_name ? ' <span class="rec-sub">' + esc(g.event_name) + '</span>' : '') +
          (g.created_by ? ' <span class="rec-sub">· ' + esc(g.created_by) + '</span>' : '') + '</div>' +
          (g.note ? '<div class="rec-sub">' + esc(g.note) + '</div>' : '') + '</div>' +
          '<span class="rec-amt ' + g.direction + '">' + (g.direction === 'in' ? '+' : '-') + fmt(g.amount_cents) + '</span>' +
          '<span class="rec-sub" style="color:#39414f">›</span></div>';
      }).join('');
      list.querySelectorAll('.rec[data-gid]').forEach(function (r) {
        r.onclick = function () { editGiftSheet(+r.getAttribute('data-gid')); };
      });
    }).catch(function () { });
  }

  /* ---- 编辑一笔往来 ---- */
  function editGiftSheet(id) {
    api('/api/gifts?limit=200').then(function (j) {
      var g = (j.records || []).filter(function (x) { return x.id === id; })[0];
      if (!g) { toast('找不到这笔记录'); return; }
      return api('/api/events').then(function (ev) {
        var events = ev.events || [];
        F.openSheet('编辑这笔往来',
          '<div class="mode-switch" id="edDir"><button class="mode' + (g.direction === 'in' ? ' on' : '') + '" data-d="in">收到礼金</button>' +
          '<button class="mode' + (g.direction === 'out' ? ' on' : '') + '" data-d="out">送出礼金</button></div>' +
          '<div class="field"><span class="field-lb">姓名</span><input id="edName" class="field-in" value="' + esc(g.person_name) + '"></div>' +
          '<div class="field"><span class="field-lb">金额</span><input id="edAmt" class="field-in" inputmode="decimal" value="' + (g.amount_cents / 100) + '"></div>' +
          '<div class="field"><span class="field-lb">日期</span><input id="edDate" class="field-in" type="date" value="' + esc(g.occurred_on) + '"></div>' +
          '<div class="field"><span class="field-lb">事由</span><input id="edEvt" class="field-in" list="evtList" value="' + esc(g.event_name || '') + '"></div>' +
          '<datalist id="evtList">' + events.map(function (e) { return '<option value="' + esc(e.name) + '">'; }).join('') + '</datalist>' +
          '<div class="field"><span class="field-lb">备注</span><input id="edNote" class="field-in" value="' + esc(g.note || '') + '"></div>' +
          '<div style="display:flex;gap:8px;margin-top:6px">' +
          '<button class="btn" id="edSkip" style="flex:1">标记无需回礼</button>' +
          '<button class="btn btn-primary" id="edSave" style="flex:1">保 存</button></div>' +
          '<button class="btn" id="edDel" style="width:100%;margin-top:8px;color:var(--in)">删除这笔记录</button>');
        var dir = g.direction;
        $('sheetBody').querySelectorAll('#edDir .mode').forEach(function (b) {
          b.onclick = function () {
            dir = b.getAttribute('data-d');
            $('sheetBody').querySelectorAll('#edDir .mode').forEach(function (x) { x.classList.toggle('on', x === b); });
          };
        });
        $('edSave').onclick = function () {
          var body = {
            direction: dir, amount: $('edAmt').value, date: $('edDate').value,
            note: $('edNote').value.trim()
          };
          var nm = $('edName').value.trim();
          if (nm !== g.person_name) {
            api('/api/persons', { method: 'POST', body: { name: nm } }).then(function (r) {
              if (r.person) body.person_id = r.person.id;
              send();
            }).catch(send);
          } else send();
          function send() {
            var en = $('edEvt').value.trim();
            if (en !== (g.event_name || '')) {
              api('/api/events', { method: 'POST', body: { name: en } }).then(function (r) {
                if (r.id) body.event_id = r.id;
                patch(body);
              }).catch(function () { patch(body); });
            } else patch(body);
          }
          function patch(b) {
            api('/api/gifts/' + id, { method: 'PATCH', body: b }).then(function () {
              F.closeSheet(); toast('已更新'); gift(); F.loadInsights();
            }).catch(function (e) { toast(e.message); });
          }
        };
        $('edSkip').onclick = function () {
          api('/api/gifts/' + id, { method: 'PATCH', body: { settle: 'done' } }).then(function () {
            F.closeSheet(); toast('已标记，不再提醒'); gift();
          }).catch(function (e) { toast(e.message); });
        };
        $('edDel').onclick = function () {
          F.confirmSheet('删除这笔往来？', '删了就找不回来了', '删除', function () {
            api('/api/gifts/' + id, { method: 'DELETE' }).then(function () { gift(); F.loadInsights(); F.loadRecent(); });
          });
        };
      });
    }).catch(function (e) { toast(e.message); });
  }

  /* ---- 筛选 ---- */
  function filterSheet() {
    var RELS = ['亲戚', '同事', '同学', '朋友', '邻居', '其他'];
    F.openSheet('筛选往来',
      '<div class="card-t" style="margin-bottom:8px">关系</div>' +
      '<div class="cat-grid" id="flRel">' + RELS.map(function (r) {
        return '<button class="cat' + (curFilter.relation === r ? ' on' : '') + '" data-v="' + r + '">' + r + '</button>';
      }).join('') + '</div>' +
      '<div class="card-t" style="margin:12px 0 8px">方向</div>' +
      '<div class="cat-grid" id="flDir">' +
      '<button class="cat' + (curFilter.dir === 'in' ? ' on' : '') + '" data-v="in">收到</button>' +
      '<button class="cat' + (curFilter.dir === 'out' ? ' on' : '') + '" data-v="out">送出</button></div>' +
      '<div class="field" style="margin-top:12px"><span class="field-lb">关键词</span>' +
      '<input id="flQ" class="field-in" placeholder="姓名 / 事由 / 备注" value="' + esc(curFilter.q || '') + '"></div>' +
      '<div style="display:flex;gap:8px;margin-top:6px">' +
      '<button class="btn" id="flClear" style="flex:1">清空条件</button>' +
      '<button class="btn btn-primary" id="flOk" style="flex:1">应 用</button></div>');
    var rel = curFilter.relation || '', dir = curFilter.dir || '';
    $('sheetBody').querySelectorAll('#flRel .cat').forEach(function (b) {
      b.onclick = function () {
        var v = b.getAttribute('data-v');
        rel = (rel === v) ? '' : v;
        $('sheetBody').querySelectorAll('#flRel .cat').forEach(function (x) { x.classList.toggle('on', x.getAttribute('data-v') === rel); });
      };
    });
    $('sheetBody').querySelectorAll('#flDir .cat').forEach(function (b) {
      b.onclick = function () {
        var v = b.getAttribute('data-v');
        dir = (dir === v) ? '' : v;
        $('sheetBody').querySelectorAll('#flDir .cat').forEach(function (x) { x.classList.toggle('on', x.getAttribute('data-v') === dir); });
      };
    });
    $('flClear').onclick = function () { curFilter = {}; F.closeSheet(); gift(); };
    $('flOk').onclick = function () {
      curFilter = {};
      if (rel) curFilter.relation = rel;
      if (dir) curFilter.dir = dir;
      var q = $('flQ').value.trim();
      if (q) curFilter.q = q;
      F.closeSheet(); gift();
    };
  }

  /* ---- 批量录礼单 ---- */
  function batchSheet(eventID, eventName) {
    batchSheet.eventID = eventID || 0;
    batchSheet.eventName = eventName || '';
    F.openSheet('批量录礼单',
      '<div style="color:var(--dim);font-size:13px;margin-bottom:12px">' +
      '办酒席收了一堆红包？一行一条，名字和金额用空格或逗号隔开，一次性能录几十条。</div>' +
      '<div class="field"><span class="field-lb">事由</span>' +
      '<input id="btEvt" class="field-in" placeholder="如 宝宝周岁宴" value="' + esc(batchSheet.eventName || '') + '"></div>' +
      '<div class="field"><span class="field-lb">日期</span>' +
      '<input id="btDate" class="field-in" type="date" value="' + F.todayStr() + '"></div>' +
      '<textarea id="btText" style="width:100%;height:150px;background:var(--card);border:1px solid var(--line);' +
      'border-radius:12px;padding:12px;color:var(--text);font-size:14px;resize:none" ' +
      'placeholder="王姨 2000&#10;李哥 800&#10;张姐 500&#10;刘叔 1000"></textarea>' +
      '<div id="btPreview" style="font-size:12px;color:var(--dim2);margin:8px 0"></div>' +
      '<button class="save" id="btGo">录 入</button>');
    function preview() {
      var items = parseBatch($('btText').value);
      var sum = items.reduce(function (a, b) { return a + b.cents; }, 0);
      $('btPreview').innerHTML = items.length
        ? '识别到 <b style="color:var(--accent)">' + items.length + '</b> 条，合计 ¥' + fmt(sum)
        : '还没输入内容';
      return items;
    }
    $('btText').oninput = preview;
    preview();
    $('btGo').onclick = function () {
      var items = parseBatch($('btText').value);
      if (!items.length) { toast('没有识别到内容'); return; }
      api('/api/gifts/batch', {
        method: 'POST',
        body: {
          event_id: batchSheet.eventID || 0, event_name: $('btEvt').value.trim(), date: $('btDate').value,
          items: items.map(function (i) { return { name: i.name, amount: String(i.cents / 100) }; })
        }
      }).then(function (r) {
        F.closeSheet();
        toast('已录入 ' + r.count + ' 笔礼金');
        curRange = 'all';
        gift(); F.loadInsights(); F.loadRecent();
      }).catch(function (e) { toast(e.message); });
    };
  }
  batchSheet.eventID = 0;
  batchSheet.eventName = '';

  // 解析 "王姨 2000" / "王姨,2000" / "王姨 2,000"
  function parseBatch(text) {
    var out = [];
    String(text || '').split(/\n+/).forEach(function (line) {
      line = line.trim();
      if (!line) return;
      var m = line.match(/^(.*?)[\s,，、\t]+([0-9][0-9,\.]*)\s*$/);
      if (!m) return;
      var name = m[1].trim();
      var amt = parseFloat(String(m[2]).replace(/,/g, ''));
      if (!name || !amt || isNaN(amt)) return;
      out.push({ name: name, cents: Math.round(amt * 100) });
    });
    return out;
  }

  function personSheet(id) {
    F.openSheet('往来明细', '<div class="empty">加载中…</div>');
    api('/api/persons/' + id + '/records').then(function (j) {
      var p = j.person, arr = j.records || [];
      var n = p.net_cents;
      var html = '<div class="stat-grid"><div class="stat"><div class="stat-lb">收到</div><div class="stat-v in">¥' + fmt(p.recv_cents) + '</div></div>' +
        '<div class="stat"><div class="stat-lb">送出</div><div class="stat-v out">¥' + fmt(p.sent_cents) + '</div></div></div>';
      html += '<div class="card" style="margin-top:12px"><div class="card-h"><span class="card-t">' + esc(p.name) + '</span></div>' +
        '<div style="font-size:13px;color:var(--dim)">' + (n > 0 ? '你还欠 ' + fmt(n) : (n < 0 ? '对方欠 ' + fmt(-n) : '双方持平')) + '</div></div>';
      html += '<div class="list-h"><span class="list-t">全部往来</span></div>';
      if (!arr.length) html += '<div class="empty">还没有记录</div>';
      else html += arr.map(function (g) {
        return '<div class="rec"><span class="rec-dt">' + esc(F.dayLabel(g.occurred_on)) + '</span>' +
          '<div class="rec-mid"><div class="rec-nm">' + esc(g.event_name || '日常往来') +
          (g.created_by ? ' <span class="rec-sub">· ' + esc(g.created_by) + '</span>' : '') + '</div></div>' +
          '<span class="rec-amt ' + g.direction + '">' + (g.direction === 'in' ? '+' : '-') + fmt(g.amount_cents) + '</span>' +
          '<button class="rec-del" data-id="' + g.id + '">×</button></div>';
      }).join('');
      html += '<button class="btn btn-primary" id="quickBack" style="width:100%;margin-top:10px;padding:13px">记一笔回礼</button>';
      $('sheetBody').innerHTML = '<div class="sheet-bar"></div><div class="sheet-t">' + esc(p.name) +
        '<button class="sheet-close" id="sheetX">关闭</button></div>' + html;
      $('sheetX').onclick = F.closeSheet;
      $('sheetBody').querySelectorAll('.rec-del').forEach(function (b) {
        b.onclick = function () {
          F.confirmSheet('删除？', '', '删除', function () {
            api('/api/gifts/' + b.getAttribute('data-id'), { method: 'DELETE' }).then(function () { gift(); F.loadInsights(); F.closeSheet(); });
          });
        };
      });
      $('quickBack').onclick = function () {
        F.closeSheet();
        F.nav('home');
        setTimeout(function () {
          document.querySelector('.mode[data-mode="gift"]').click();
          if (document.querySelector('.dir-btn[data-dir="out"]')) document.querySelector('.dir-btn[data-dir="out"]').click();
          $('who').value = p.name;
          $('who').dispatchEvent(new Event('input'));
          $('amt').focus();
        }, 60);
      };
    }).catch(function (e) { $('sheetBody').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }

  /* ============ 成长 ============ */
  function grow() {
    var box = $('growOverview'), list = $('babyList'), ms = $('msList'), sup = $('supplyBox');
    box.innerHTML = '<div class="family-actions grow-tools">' + F.babyPickerHTML('growBabyPicker') + '<button class="btn btn-primary" id="addBabyProfile">＋ 添加宝宝</button></div><div class="card"><div class="skel" style="height:90px"></div></div>';
    F.refreshBabyPickers();
    $('addBabyProfile').onclick = addBabyProfileSheet;
    if ($('growBabyPicker')) $('growBabyPicker').onchange = function () { S.babyID = Number(this.value) || 1; localStorage.setItem('fl_baby_id', String(S.babyID)); api('/api/babies/' + S.babyID + '/activate', { method: 'POST' }).catch(function () { }); document.querySelectorAll('.baby-picker').forEach(function (x) { x.value = String(S.babyID); }); grow(); };
    api(F.babyQuery('/api/baby/summary?months=12')).then(function (j) {
      var s = j.summary;
      return api(F.babyQuery('/api/stats?range=' + curRange)).then(function (st) {
      return api(F.babyQuery('/api/insights')).then(function (ins) {
        var v = ins.insights, stt = st.stats;
        box.innerHTML = '<div class="family-actions grow-tools">' + F.babyPickerHTML('growBabyPicker') + '<button class="btn btn-primary" id="addBabyProfile">＋ 添加宝宝</button></div>';
        var html = rangeHTML();
        html += '<div class="stat-grid">' +
          '<div class="stat"><div class="stat-lb">本月开销</div><div class="stat-v warn">¥' + fmt(s.cur_month_cents) + '</div></div>' +
          '<div class="stat"><div class="stat-lb">累计养育</div><div class="stat-v">¥' + fmt(s.total_cents) + '</div>' +
          '<div class="stat-x">' + s.count + ' 笔记录</div></div>' +
          '<div class="stat"><div class="stat-lb">本期开销</div><div class="stat-v">¥' + fmt(stt.baby_total) + '</div>' +
          '<div class="stat-x">' + deltaHTML(stt.baby_total, stt.prev_baby_total) + '</div></div>' +
          '<div class="stat"><div class="stat-lb">本期人情</div><div class="stat-v ' + (stt.gift_net > 0 ? 'in' : 'out') + '">¥' + fmt(stt.gift_net) + '</div>' +
          '<div class="stat-x">收 ' + fmt(stt.gift_recv) + ' / 送 ' + fmt(stt.gift_sent) + '</div></div>';
        if (v.project_year_cents) {
          html += '<div class="stat wide"><div class="stat-lb">按最近花销推算</div>' +
            '<div class="stat-v acc">一年约 ¥' + fmt(v.project_year_cents) + '</div>' +
            (v.forecast_to_3y ? '<div class="stat-x">到宝宝 3 岁累计约 ¥' + fmt(v.forecast_to_3y) + '</div>' : '') + '</div>';
        }
        html += '</div>';

        // 分类环形图
        var cats = (s.categories || []).slice(0, 8);
        if (cats.length) {
          html += '<div class="card" style="margin-top:12px"><div class="card-h"><span class="card-t">钱都花在哪</span></div>' +
            '<div class="ring-wrap"><canvas id="ring"></canvas><div class="ring-legend">' +
            cats.map(function (c, i) {
              return '<div class="lg-row"><span class="lg-dot" style="background:' + PALETTE[i % PALETTE.length] + '"></span>' +
                '<span class="lg-nm">' + esc(c.category) + '</span><span class="lg-v">¥' + fmt(c.amount_cents) + '</span></div>';
            }).join('') + '</div></div></div>';
        }

        // 按月趋势
        if ((s.months || []).length) {
          var mm = s.months.slice(0, 6).reverse();
          var max = 1;
          mm.forEach(function (m) { if (m.amount_cents > max) max = m.amount_cents; });
          html += '<div class="card"><div class="card-h"><span class="card-t">近半年走势</span></div>' +
            mm.map(function (m) {
              return '<div class="bar-row"><span class="bar-lb">' + esc(m.month.slice(2)) + '</span>' +
                '<span class="bar-bg"><span class="bar-fl" style="width:' + Math.round(m.amount_cents / max * 100) + '%"></span></span>' +
                '<span class="bar-v">¥' + fmt(m.amount_cents) + '</span></div>';
            }).join('') + '</div>';
        }
        box.innerHTML = html;
        box.insertAdjacentHTML('afterbegin', '<div class="family-actions grow-tools">' + F.babyPickerHTML('growBabyPicker') + '<button class="btn btn-primary" id="addBabyProfile">＋ 添加宝宝</button></div>');
        F.refreshBabyPickers();
        $('addBabyProfile').onclick = addBabyProfileSheet;
        if ($('growBabyPicker')) $('growBabyPicker').onchange = function () { S.babyID = Number(this.value) || 1; localStorage.setItem('fl_baby_id', String(S.babyID)); api('/api/babies/' + S.babyID + '/activate', { method: 'POST' }).catch(function () { }); grow(); };
        box.querySelectorAll('#rangeSwitch .mode').forEach(function (b) {
          b.onclick = function () { curRange = b.getAttribute('data-r'); grow(); };
        });
        if (cats.length) {
          var data = cats.map(function (c, i) { return { value: c.amount_cents, color: PALETTE[i % PALETTE.length] }; });
          drawRing($('ring'), data, s.categories.reduce(function (a, b) { return a + b.amount_cents; }, 0));
        }

        // 日历热力图：一眼看出哪天花得多
        if ((stt.days || []).length) {
          var hb = document.createElement('div');
          hb.className = 'card';
          hb.style.marginTop = '12px';
          hb.innerHTML = heatHTML(stt.days);
          box.appendChild(hb);
        }

        // 补货提醒
        var soon = (v.consumables || []).filter(function (c) { return c.days_left <= 45; });
        if (soon.length) {
          sup.innerHTML = '<div class="card"><div class="card-h"><span class="card-t">按消耗速度推算</span></div>' +
            soon.map(function (c) {
              var color = c.days_left <= 3 ? 'var(--in)' : (c.days_left <= 10 ? 'var(--warn)' : 'var(--out)');
              return '<div class="alert ' + (c.days_left <= 3 ? 'urgent' : (c.days_left <= 10 ? 'warn' : 'info')) + '">' +
                '<span class="dot"></span><div class="alert-b"><div class="alert-t">' + esc(c.category) +
                ' 约 ' + c.days_left + ' 天后见底</div>' +
                '<div class="alert-d">日均 ' + c.daily_use + ' ' + esc(c.unit) + '，最近买了 ' + c.last_qty +
                ' ' + esc(c.unit) + '（' + esc(c.last_on) + '）· 均价 ¥' + fmt(c.avg_price) + '</div></div></div>';
            }).join('') + '</div>';
        } else sup.innerHTML = '';
      });
      });
    }).catch(function (e) { box.innerHTML = '<div class="card"><div class="empty">' + esc(e.message) + '</div></div>'; });

    // 里程碑
    ms.innerHTML = '<div class="skel" style="height:56px"></div>';
    api(F.babyQuery('/api/milestones')).then(function (j) {
      var arr = j.milestones || [];
      if (!arr.length) { ms.innerHTML = '<div class="empty">还没有记录成长节点</div>'; return; }
      ms.innerHTML = arr.map(function (m) {
        var age = S.profile && S.profile.baby_birthday ? F.ageText(S.profile.baby_birthday, m.occurred_on) : '';
        return '<div class="ms-item" data-id="' + m.id + '"><div class="ms-ic">' + esc((m.title || m.kind).slice(0, 1)) + '</div>' +
          '<div class="ms-b"><div class="ms-t">' + esc(m.title) + '</div>' +
          '<div class="ms-d">' + esc(m.occurred_on) + (age ? ' · <span class="ms-age">' + esc(age) + '</span>' : '') +
          (m.note ? ' · ' + esc(m.note) : '') + '</div></div>' +
          '<button class="rec-del" data-del="' + m.id + '">×</button></div>';
      }).join('');
      ms.querySelectorAll('[data-del]').forEach(function (b) {
        b.onclick = function (e) {
          e.stopPropagation();
          F.confirmSheet('删除这个成长记录？', esc(m_title(b)), '删除', function () {
            api('/api/milestones/' + b.getAttribute('data-del'), { method: 'DELETE' }).then(function () { grow(); F.loadRecent(); });
          });
        };
      });
      function m_title(el) { return ''; }
    }).catch(function () { });

    // 明细
    list.innerHTML = '<div class="skel" style="height:60px"></div>';
    api(F.babyQuery('/api/baby?limit=60')).then(function (j) {
      var arr = j.records || [];
      if (!arr.length) { list.innerHTML = '<div class="empty">还没有开销记录</div>'; return; }
      list.innerHTML = arr.map(function (b) {
        var qty = b.qty ? ' <span class="rec-sub">' + b.qty + (b.unit || '') + '</span>' : '';
        return '<div class="rec"><span class="rec-dt">' + esc(F.dayLabel(b.occurred_on)) + '</span>' +
          '<div class="rec-mid"><div class="rec-nm">' + esc(b.category) + qty +
          (b.merchant ? ' <span class="rec-sub">' + esc(b.merchant) + '</span>' : '') +
          (b.created_by ? ' <span class="rec-sub">· ' + esc(b.created_by) + '</span>' : '') + '</div>' +
          (b.note ? '<div class="rec-sub">' + esc(b.note) + '</div>' : '') + '</div>' +
          '<span class="rec-amt out">-' + fmt(b.amount_cents) + '</span>' +
          '<button class="rec-del" data-id="' + b.id + '">×</button></div>';
      }).join('');
      list.querySelectorAll('.rec-del').forEach(function (b) {
        b.onclick = function () {
          F.confirmSheet('删除这笔开销？', '', '删除', function () {
            api('/api/baby/' + b.getAttribute('data-id'), { method: 'DELETE' }).then(function () { grow(); F.loadInsights(); });
          });
        };
      });
    }).catch(function () { });
    loadHealthAndReminders();
  }

  function loadHealthAndReminders() {
    if (!$('healthList')) return;
    api(F.babyQuery('/api/health')).then(function (j) {
      var arr = j.records || [];
      $('healthList').innerHTML = arr.length ? arr.map(function (h) {
        return '<div class="rec"><span class="rec-dt">' + esc(h.occurred_on) + '</span><div class="rec-mid"><div class="rec-nm">' + esc(h.title) + '</div><div class="rec-sub">' + esc(h.kind) + (h.value ? ' · ' + esc(h.value) : '') + (h.note ? ' · ' + esc(h.note) : '') + '</div></div><button class="rec-del" data-health="' + h.id + '">×</button></div>';
      }).join('') : '<div class="empty">还没有健康记录</div>';
      $('healthList').querySelectorAll('[data-health]').forEach(function (b) { b.onclick = function () { F.confirmSheet('删除这条健康记录？', '', '删除', function () { api('/api/health/' + b.getAttribute('data-health'), { method: 'DELETE' }).then(loadHealthAndReminders); }); }; });
    }).catch(function (e) { $('healthList').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
    api(F.babyQuery('/api/reminders')).then(function (j) {
      var arr = j.reminders || [];
      $('reminderList').innerHTML = arr.length ? arr.map(function (x) {
        return '<div class="task-row"><button class="task-check" data-reminder="' + x.id + '">' + (x.status === 'done' ? '✓' : '○') + '</button><div class="task-body"><div class="task-title">' + esc(x.title) + '</div><div class="rec-sub">' + esc(x.kind) + ' · ' + esc(x.due_on) + (x.repeat_days ? ' · 每 ' + x.repeat_days + ' 天重複' : '') + '</div></div><button class="rec-del" data-del-reminder="' + x.id + '">×</button></div>';
      }).join('') : '<div class="empty">还没有提醒</div>';
      $('reminderList').querySelectorAll('[data-reminder]').forEach(function (b) { b.onclick = function () { var x = arr.filter(function (v) { return v.id === +b.getAttribute('data-reminder'); })[0]; api('/api/reminders/' + x.id, { method: 'PATCH', body: { status: x.status === 'done' ? 'open' : 'done' } }).then(loadHealthAndReminders); }; });
      $('reminderList').querySelectorAll('[data-del-reminder]').forEach(function (b) { b.onclick = function () { api('/api/reminders/' + b.getAttribute('data-del-reminder'), { method: 'DELETE' }).then(loadHealthAndReminders); }; });
    }).catch(function (e) { $('reminderList').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
    $('lnkAddHealth').onclick = addHealthSheet;
    $('lnkAddReminder').onclick = addReminderSheet;
  }

  function addBabyProfileSheet() {
    F.openSheet('添加宝宝档案', '<div class="field"><span class="field-lb">昵称</span><input id="newBabyName" class="field-in" placeholder="如 小宝"></div><div class="field"><span class="field-lb">生日</span><input id="newBabyBirthday" class="field-in" type="date"></div><button class="save" id="newBabySave">保存宝宝</button>');
    $('newBabySave').onclick = function () { api('/api/babies', { method: 'POST', body: { name: $('newBabyName').value, birthday: $('newBabyBirthday').value } }).then(function () { return api('/api/babies'); }).then(function (j) { S.babies = j.babies || []; S.babyID = S.babies[S.babies.length - 1].id; localStorage.setItem('fl_baby_id', String(S.babyID)); api('/api/babies/' + S.babyID + '/activate', { method: 'POST' }).catch(function () { }); F.closeSheet(); grow(); }).catch(function (e) { toast(e.message); }); };
  }

  function addHealthSheet() {
    F.openSheet('添加健康记录', F.babyPickerHTML('healthBabyPicker') + '<div class="field"><span class="field-lb">类型</span><input id="healthKind" class="field-in" value="体检"></div><div class="field"><span class="field-lb">内容</span><input id="healthTitle" class="field-in" placeholder="如 身高体重"></div><div class="field"><span class="field-lb">日期</span><input id="healthDate" class="field-in" type="date" value="' + F.todayStr() + '"></div><div class="field"><span class="field-lb">数值 / 备注</span><input id="healthValue" class="field-in" placeholder="选填，如 8.2kg"></div><button class="save" id="healthSave">保存记录</button>');
    $('healthSave').onclick = function () { api('/api/health', { method: 'POST', body: { baby_id: Number($('healthBabyPicker').value) || S.babyID, kind: $('healthKind').value, title: $('healthTitle').value, date: $('healthDate').value, value: $('healthValue').value } }).then(function () { F.closeSheet(); loadHealthAndReminders(); }).catch(function (e) { toast(e.message); }); };
  }

  function addReminderSheet() {
    F.openSheet('添加宝宝提醒', F.babyPickerHTML('remBabyPicker') + '<div class="field"><span class="field-lb">提醒内容</span><input id="remTitle" class="field-in" placeholder="如 儿保体检"></div><div class="field"><span class="field-lb">类型</span><input id="remKind" class="field-in" value="体检"></div><div class="field"><span class="field-lb">日期</span><input id="remDue" class="field-in" type="date"></div><div class="field"><span class="field-lb">重复周期（天，可留空）</span><input id="remRepeat" class="field-in" inputmode="numeric" placeholder="例如 180"></div><button class="save" id="remSave">保存提醒</button>');
    $('remSave').onclick = function () { api('/api/reminders', { method: 'POST', body: { baby_id: Number($('remBabyPicker').value) || S.babyID, title: $('remTitle').value, kind: $('remKind').value, due: $('remDue').value, repeat_days: parseInt($('remRepeat').value, 10) || 0 } }).then(function () { F.closeSheet(); loadHealthAndReminders(); }).catch(function (e) { toast(e.message); }); };
  }

  function addMilestone() {
    F.openSheet('记一笔成长', '<div class="empty">加载中…</div>');
    api('/api/milestone-kinds').then(function (j) {
      var kinds = j.kinds || [];
      var html = F.babyPickerHTML('msBabyPicker') + '<div class="cat-grid" id="msKinds">' + kinds.map(function (k, i) {
        return '<button class="cat' + (i === 0 ? ' on' : '') + '" data-k="' + esc(k.kind) + '" data-t="' + esc(k.title) + '">' + esc(k.title) + '</button>';
      }).join('') + '</div>' +
        '<div class="field"><span class="field-lb">标题</span><input id="msTitle" class="field-in" value="' + esc(kinds[0] ? kinds[0].title : '') + '"></div>' +
        '<div class="field"><span class="field-lb">日期</span><input id="msDate" class="field-in" type="date" value="' + F.todayStr() + '"></div>' +
        '<div class="field"><span class="field-lb">备注</span><input id="msNote" class="field-in" placeholder="当时的情景，选填"></div>' +
        '<button class="save" id="msSave">记 下 来</button>';
      $('sheetBody').innerHTML = '<div class="sheet-bar"></div><div class="sheet-t">成长里程碑' +
        '<button class="sheet-close" id="sheetX">关闭</button></div>' + html;
      $('sheetX').onclick = F.closeSheet;
      var kind = kinds[0] ? kinds[0].kind : '';
      $('msKinds').querySelectorAll('.cat').forEach(function (b) {
        b.onclick = function () {
          $('msKinds').querySelectorAll('.cat').forEach(function (x) { x.classList.remove('on'); });
          b.classList.add('on');
          kind = b.getAttribute('data-k');
          $('msTitle').value = b.getAttribute('data-t');
        };
      });
      $('msSave').onclick = function () {
        api('/api/milestones', {
          method: 'POST',
          body: { baby_id: Number($('msBabyPicker').value) || S.babyID, kind: kind, title: $('msTitle').value.trim(), date: $('msDate').value, note: $('msNote').value.trim() }
        }).then(function () {
          F.closeSheet(); toast('已记下'); grow(); F.loadRecent();
        }).catch(function (e) { toast(e.message); });
      };
    }).catch(function (e) { $('sheetBody').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }

  /* ============ 我的 ============ */
  function me() {
    var box = $('meBox');
    box.innerHTML = '<div class="card"><div class="skel" style="height:70px"></div></div>';
    api('/api/members').then(function (j) {
      return api('/api/profile').then(function (p) {
        var pr = p.profile || {};
        S.profile = pr;
        var bd = pr.baby_birthday || '';
        var age = bd ? F.ageText(bd) : '';
        var html = '<div class="card"><div class="card-h"><span class="card-t">宝宝档案</span></div>' +
          '<div class="setting-row" id="swName"><span class="sr-t">昵称</span><span class="sr-v">' + esc(pr.baby_name || '宝宝') + ' ›</span></div>' +
          '<div class="setting-row" id="swBd"><span class="sr-t">生日</span><span class="sr-v">' + esc(bd || '未设置') + ' ›</span></div>' +
          (age ? '<div style="font-size:12px;color:var(--accent);padding:2px 14px 6px">' + esc(age) + '</div>' : '') +
          '</div>';

        html += '<div class="card"><div class="card-h"><span class="card-t">家庭成员</span>' +
          '<span class="link" id="lnkAddMember">＋ 添加</span></div>' +
          (j.members || []).map(function (m) {
            return '<div class="setting-row"><span class="sr-t">' + esc(m.nickname) + '</span>' +
              (S.me && S.me.id === m.id ? '<span class="sr-v" style="color:var(--accent)">当前</span>' : '') + '</div>';
          }).join('') + '</div>';

        html += '<div class="card"><div class="card-h"><span class="card-t">导出</span></div>' +
          '<div class="setting-row" id="swExp"><span class="sr-t">导出成 Excel（CSV）</span><span class="sr-v">›</span></div>' +
          '<div class="setting-row" id="swBackup"><span class="sr-t">完整备份（JSON，可用于换机器）</span><span class="sr-v">›</span></div>' +
          '</div>';

        html += '<div class="card"><div class="card-h"><span class="card-t">数据</span></div>' +
          '<div class="setting-row" id="swRestore"><span class="sr-t">从备份文件恢复</span><span class="sr-v">›</span></div>' +
          '<div style="font-size:12px;color:var(--dim2);padding:0 14px 4px">' +
          '数据存在你自己的 NAS 上的单个数据库文件里，每天凌晨 3 点自动备份一次。' +
          '换服务器时，导出一份 JSON 再在新机器上恢复即可。</div></div>';

        html += '<button class="btn" id="btnLogout2" style="width:100%;padding:13px;color:var(--in)">退出登录</button>';
        box.innerHTML = html;

        $('swName').onclick = function () {
          var v = prompt('宝宝昵称', pr.baby_name || '宝宝');
          if (!v) return;
          api('/api/profile', { method: 'POST', body: { baby_name: v.trim() } }).then(function () { me(); F.loadInsights(); });
        };
        $('swBd').onclick = function () {
          var v = prompt('宝宝生日（格式 2026-05-20）', bd);
          if (v === null) return;
          api('/api/profile', { method: 'POST', body: { baby_birthday: v.trim() } }).then(function () { me(); F.loadInsights(); });
        };
        $('lnkAddMember').onclick = function () {
          var v = prompt('添加家庭成员，输入称呼', '');
          if (!v) return;
          api('/api/members', { method: 'POST', body: { nickname: v.trim() } }).then(function () { me(); toast('已添加'); })
            .catch(function (e) { toast(e.message); });
        };
        $('swExp').onclick = function () {
          F.openSheet('导出到电脑', '<div style="color:var(--dim);font-size:13px;margin-bottom:14px">导出文件用 Excel / WPS 直接打开，中文不会乱码。</div>' +
            '<button class="btn btn-primary" id="exG" style="width:100%;margin-bottom:10px;padding:13px">人情账本（红包往来）</button>' +
            '<button class="btn btn-primary" id="exB" style="width:100%;padding:13px">宝宝开销明细</button>');
          $('exG').onclick = function () { location.href = '/api/export.csv?book=gift'; F.closeSheet(); };
          $('exB').onclick = function () { location.href = '/api/export.csv?book=baby'; F.closeSheet(); };
        };
        $('btnLogout2').onclick = function () {
          F.confirmSheet('退出登录？', '这台设备上会清除登录状态，数据不受影响', '退出', function () { F.logout(); });
        };
        $('swBackup').onclick = function () { location.href = '/api/backup.json'; };
        $('swRestore').onclick = function () {
          F.openSheet('从备份恢复',
            '<div style="color:var(--dim);font-size:13px;margin-bottom:14px">' +
            '选择之前导出的 JSON 备份文件。<b style="color:var(--warn)">同名的人和事由会自动合并，记录会追加</b>，' +
            '不会覆盖现有数据。</div>' +
            '<input id="rsFile" type="file" accept="application/json,.json" ' +
            'style="width:100%;padding:12px;background:var(--card);border:1px solid var(--line);border-radius:12px;color:var(--dim)">' +
            '<button class="save" id="rsGo" style="margin-top:12px">恢复数据</button>');
          $('rsGo').onclick = function () {
            var f = $('rsFile').files && $('rsFile').files[0];
            if (!f) { toast('先选一个备份文件'); return; }
            var fr = new FileReader();
            fr.onload = function () {
              var data;
              try { data = JSON.parse(fr.result); }
              catch (e) { toast('文件不是有效的 JSON'); return; }
              api('/api/restore', { method: 'POST', body: data }).then(function (r) {
                F.closeSheet(); toast('已恢复 ' + r.count + ' 条记录'); F.nav('home');
              }).catch(function (e) { toast(e.message); });
            };
            fr.readAsText(f);
          };
        };
      });
    }).catch(function (e) { box.innerHTML = '<div class="card"><div class="empty">' + esc(e.message) + '</div></div>'; });
  }

  /* ============ 时间线弹层 ============ */
  function timelineSheet() {
    F.openSheet('全部时间线', '<div class="empty">加载中…</div>');
    api('/api/timeline?limit=200').then(function (j) {
      $('sheetBody').innerHTML = '<div class="sheet-bar"></div><div class="sheet-t">全部时间线' +
        '<button class="sheet-close" id="sheetX">关闭</button></div>' + F.timelineHTML(j.items || []);
      $('sheetX').onclick = F.closeSheet;
    }).catch(function (e) { $('sheetBody').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }

  FL.registerViews({ gift: gift, grow: grow, me: me, timelineSheet: timelineSheet, addMilestone: addMilestone, personSheet: personSheet });
  window.FLBatchSheet = batchSheet;
  window.FLBatchSheet = batchSheet;
})();
