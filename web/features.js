/* 添丁记 - 家庭协作、事件工作台、关系洞察与报告 */
(function () {
  'use strict';
  var F = window.FL, S = F.S, $ = F.$, api = F.api, esc = F.esc, fmt = F.fmt, toast = F.toast;
  var section = 'events';
  var sections = [['events', '事件'], ['tasks', '事项'], ['shopping', '采购'], ['relations', '关系'], ['report', '报告'], ['audit', '记录']];

  function family() {
    var root = $('familyBox');
    root.innerHTML = '<div class="family-tabs">' + sections.map(function (x) {
      return '<button class="family-tab' + (x[0] === section ? ' on' : '') + '" data-s="' + x[0] + '">' + x[1] + '</button>';
    }).join('') + '</div><div id="familyContent"></div>';
    root.querySelectorAll('.family-tab').forEach(function (b) {
      b.onclick = function () { section = b.getAttribute('data-s'); family(); };
    });
    if (section === 'events') eventsView();
    if (section === 'tasks') tasksView();
    if (section === 'shopping') shoppingView();
    if (section === 'relations') relationsView();
    if (section === 'report') reportView();
    if (section === 'audit') auditView();
  }

  function content() { return $('familyContent'); }
  function header(title, actionID, actionText) {
    return '<div class="family-head"><div><div class="family-title">' + title + '</div>' +
      '<div class="family-sub">所有家庭成员都能看到最新状态</div></div>' +
      (actionID ? '<button class="btn btn-primary family-action" id="' + actionID + '">' + actionText + '</button>' : '') + '</div>';
  }
  function statusText(s) { return { planned: '准备中', active: '进行中', done: '已完成' }[s] || '准备中'; }

  function eventsView() {
    content().innerHTML = header('重要事件', 'addEvent', '＋ 新建事件') + '<div id="eventList"><div class="skel" style="height:90px"></div></div>';
    $('addEvent').onclick = addEventSheet;
    api('/api/events').then(function (j) {
      var arr = j.events || [], list = $('eventList');
      if (!arr.length) { list.innerHTML = '<div class="empty">还没有事件，先建立一次满月宴或周岁宴</div>'; return; }
      list.innerHTML = arr.map(function (e) {
        return '<button class="event-card" data-id="' + e.id + '"><div class="event-card-top"><span class="event-name">' + esc(e.name) + '</span>' +
          '<span class="event-status ' + esc(e.status) + '">' + statusText(e.status) + '</span></div>' +
          '<div class="event-meta">' + esc(e.event_date || '日期待定') + (e.venue ? ' · ' + esc(e.venue) : '') + '</div>' +
          '<div class="event-stats"><span>' + e.count + ' 笔礼金</span>' + (e.guest_count ? '<span>计划 ' + e.guest_count + ' 人</span>' : '') + '</div></button>';
      }).join('');
      list.querySelectorAll('[data-id]').forEach(function (b) { b.onclick = function () { eventSheet(+b.getAttribute('data-id')); }; });
    }).catch(function (e) { $('eventList').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }

  function addEventSheet() {
    F.openSheet('新建家庭事件', '<div class="field"><span class="field-lb">名称</span><input id="evName" class="field-in" placeholder="如 宝宝周岁宴"></div>' +
      '<div class="field"><span class="field-lb">日期</span><input id="evDate" class="field-in" type="date" value="' + F.todayStr() + '"></div>' +
      '<div class="field"><span class="field-lb">地点</span><input id="evVenue" class="field-in" placeholder="选填"></div>' +
      '<div class="row2"><div class="field half"><span class="field-lb">人数</span><input id="evGuests" class="field-in" inputmode="numeric" placeholder="选填"></div>' +
      '<div class="field half"><span class="field-lb">预算</span><input id="evBudget" class="field-in" inputmode="decimal" placeholder="选填"></div></div>' +
      '<div class="field"><span class="field-lb">备注</span><input id="evNote" class="field-in" placeholder="选填"></div>' +
      '<button class="save" id="evSave">建立事件</button>');
    $('evSave').onclick = function () {
      var name = $('evName').value.trim();
      if (!name) { toast('先填写事件名称'); return; }
      api('/api/events', { method: 'POST', body: { name: name, date: $('evDate').value, venue: $('evVenue').value.trim(),
        guest_count: parseInt($('evGuests').value, 10) || 0, budget: $('evBudget').value.trim(), note: $('evNote').value.trim(), status: 'planned' } })
        .then(function () { F.closeSheet(); toast('事件已建立'); eventsView(); }).catch(function (e) { toast(e.message); });
    };
  }

  function eventSheet(id) {
    F.openSheet('事件工作台', '<div class="empty">加载中…</div>');
    Promise.all([api('/api/events/' + id), api('/api/attachments?kind=event&record_id=' + id)]).then(function (all) {
      var d = all[0].detail, files = all[1].attachments || [];
      var html = '<div class="event-hero"><div class="event-card-top"><span class="event-name">' + esc(d.event.name) + '</span>' +
        '<span class="event-status ' + esc(d.event.status) + '">' + statusText(d.event.status) + '</span></div>' +
        '<div class="event-meta">' + esc(d.event.event_date || '日期待定') + (d.event.venue ? ' · ' + esc(d.event.venue) : '') + '</div>' +
        '<div class="stat-grid"><div class="stat"><div class="stat-lb">收到礼金</div><div class="stat-v in">¥' + fmt(d.recv_cents) + '</div></div>' +
        '<div class="stat"><div class="stat-lb">送出礼金</div><div class="stat-v out">¥' + fmt(d.sent_cents) + '</div></div>' +
        '<div class="stat"><div class="stat-lb">待回礼</div><div class="stat-v warn">¥' + fmt(d.pending_cents) + '</div></div>' +
        '<div class="stat"><div class="stat-lb">待感谢</div><div class="stat-v acc">' + d.thanks_pending + ' 人</div></div></div></div>';
      html += '<div class="family-subhead"><span>礼单</span><button class="link" id="eventBatch">＋ 批量录入</button></div>';
      html += d.gifts.length ? d.gifts.map(function (g) {
        var thanks = g.thanks_status === 'sent' ? '已感谢' : (g.thanks_status === 'skipped' ? '无需感谢' : '待感谢');
        return '<div class="rec"><span class="rec-dt">' + esc(g.occurred_on) + '</span><div class="rec-mid"><div class="rec-nm">' + esc(g.person_name) + '</div>' +
          '<div class="rec-sub">' + esc(thanks) + (g.settle ? ' · 已处理回礼' : '') + '</div></div>' +
          '<span class="rec-amt ' + g.direction + '">' + (g.direction === 'in' ? '+' : '-') + fmt(g.amount_cents) + '</span></div>';
      }).join('') : '<div class="empty">还没有礼金记录</div>';
      html += '<div class="family-subhead"><span>照片与凭证</span><label class="link upload-label" for="eventFile">＋ 添加照片</label></div>' +
        '<input id="eventFile" type="file" accept="image/*,application/pdf" class="hidden">' +
        '<div class="attachment-list">' + (files.length ? files.map(function (f) { return '<a class="attachment" target="_blank" href="/api/attachments/' + f.id + '">' + esc(f.file_name) + '</a>'; }).join('') : '<span class="rec-sub">把红包封面、小票或合照放在这个事件里</span>') + '</div>';
      html += '<div class="family-actions"><button class="btn" id="eventPrint">打印礼单</button><button class="btn" id="eventDone">标记已完成</button></div>';
      html += '<button class="link event-edit-link" id="eventEdit">编辑事件资料</button>';
      $('sheetBody').innerHTML = '<div class="sheet-bar"></div><div class="sheet-t">事件工作台<button class="sheet-close" id="sheetX">关闭</button></div>' + html;
      $('sheetX').onclick = F.closeSheet;
      $('eventBatch').onclick = function () {
        var e = d.event;
        F.closeSheet();
        if (typeof window.FLBatchSheet === 'function') window.FLBatchSheet(e.id, e.name);
      };
      $('eventEdit').onclick = function () { editEventSheet(d.event); };
      $('eventPrint').onclick = function () { window.print(); };
      $('eventDone').onclick = function () { api('/api/events/' + id, { method: 'PATCH', body: { status: 'done' } }).then(function () { F.closeSheet(); eventsView(); toast('事件已完成'); }); };
      $('eventFile').onchange = function () { uploadAttachment('event', id, $('eventFile').files[0]); };
    }).catch(function (e) { $('sheetBody').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }

  function editEventSheet(e) {
    F.openSheet('编辑事件资料', '<div class="field"><span class="field-lb">名称</span><input id="editEvName" class="field-in" value="' + esc(e.name) + '"></div>' +
      '<div class="field"><span class="field-lb">日期</span><input id="editEvDate" class="field-in" type="date" value="' + esc(e.event_date || '') + '"></div>' +
      '<div class="field"><span class="field-lb">地点</span><input id="editEvVenue" class="field-in" value="' + esc(e.venue || '') + '"></div>' +
      '<div class="field"><span class="field-lb">预算</span><input id="editEvBudget" class="field-in" inputmode="decimal" value="' + (e.budget_cents ? fmt(e.budget_cents) : '') + '"></div>' +
      '<div class="field"><span class="field-lb">计划人数</span><input id="editEvGuests" class="field-in" inputmode="numeric" value="' + (e.guest_count || '') + '"></div>' +
      '<div class="field"><span class="field-lb">备注</span><input id="editEvNote" class="field-in" value="' + esc(e.note || '') + '"></div><button class="save" id="editEvSave">保存修改</button>');
    $('editEvSave').onclick = function () { api('/api/events/' + e.id, { method: 'PATCH', body: { name: $('editEvName').value, date: $('editEvDate').value, venue: $('editEvVenue').value, budget: $('editEvBudget').value, budget_set: true, guest_count: parseInt($('editEvGuests').value, 10) || 0, guest_count_set: true, note: $('editEvNote').value, status: e.status } }).then(function () { F.closeSheet(); eventSheet(e.id); eventsView(); toast('事件资料已更新'); }).catch(function (x) { toast(x.message); }); };
  }

  function uploadAttachment(kind, id, file) {
    if (!file) return;
    if (file.size > 10 * 1024 * 1024) { toast('附件不能超过 10MB'); return; }
    var reader = new FileReader();
    reader.onload = function () {
      api('/api/attachments', { method: 'POST', body: { kind: kind, record_id: id, file_name: file.name, mime: file.type || 'application/octet-stream', data: reader.result } })
        .then(function () { toast('附件已保存'); eventSheet(id); }).catch(function (e) { toast(e.message); });
    };
    reader.readAsDataURL(file);
  }

  function tasksView() {
    content().innerHTML = header('家庭事项', 'addTask', '＋ 新建事项') + '<div id="taskList"><div class="skel" style="height:90px"></div></div>';
    $('addTask').onclick = addTaskSheet;
    api('/api/tasks?status=open').then(function (j) {
      var arr = j.tasks || [], list = $('taskList');
      if (!arr.length) { list.innerHTML = '<div class="empty">没有待办事项</div>'; return; }
      list.innerHTML = arr.map(function (t) { return taskHTML(t); }).join(''); bindTasks(list);
    }).catch(function (e) { $('taskList').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }
  function taskHTML(t) { return '<div class="task-row"><button class="task-check" data-task="' + t.id + '" title="完成">○</button><div class="task-body"><div class="task-title">' + esc(t.title) + '</div><div class="rec-sub">' + esc(t.kind) + (t.due_on ? ' · 截止 ' + esc(t.due_on) : '') + (t.assignee ? ' · ' + esc(t.assignee) : '') + '</div></div><button class="rec-del task-del" data-del-task="' + t.id + '">×</button></div>'; }
  function bindTasks(root) {
    root.querySelectorAll('[data-task]').forEach(function (b) { b.onclick = function () { api('/api/tasks/' + b.getAttribute('data-task'), { method: 'PATCH', body: { status: 'done' } }).then(tasksView); }; });
    root.querySelectorAll('[data-del-task]').forEach(function (b) { b.onclick = function () { api('/api/tasks/' + b.getAttribute('data-del-task'), { method: 'DELETE' }).then(tasksView); }; });
  }
  function addTaskSheet() {
    F.openSheet('新建家庭事项', '<div class="field"><span class="field-lb">事项</span><input id="taskTitle" class="field-in" placeholder="如 预约宝宝体检"></div>' +
      '<div class="field"><span class="field-lb">类型</span><input id="taskKind" class="field-in" value="家庭事项"></div>' +
      '<div class="field"><span class="field-lb">截止</span><input id="taskDue" class="field-in" type="date"></div>' +
      '<div class="field"><span class="field-lb">备注</span><input id="taskNote" class="field-in" placeholder="选填"></div><button class="save" id="taskSave">添加事项</button>');
    $('taskSave').onclick = function () { api('/api/tasks', { method: 'POST', body: { title: $('taskTitle').value, kind: $('taskKind').value, due: $('taskDue').value, note: $('taskNote').value } }).then(function () { F.closeSheet(); tasksView(); toast('已添加'); }).catch(function (e) { toast(e.message); }); };
  }

  function shoppingView() {
    content().innerHTML = header('采购清单', 'addShop', '＋ 添加采购') + '<div id="shopList"><div class="skel" style="height:90px"></div></div>';
    $('addShop').onclick = addShopSheet;
    api('/api/shopping').then(function (j) {
      var arr = j.items || [], list = $('shopList');
      if (!arr.length) { list.innerHTML = '<div class="empty">采购清单还是空的</div>'; return; }
      list.innerHTML = arr.map(function (x) { return '<div class="shop-row' + (x.checked ? ' checked' : '') + '"><button class="task-check" data-shop="' + x.id + '">' + (x.checked ? '✓' : '○') + '</button><div class="task-body"><div class="task-title">' + esc(x.name) + '</div><div class="rec-sub">' + (x.qty ? x.qty + ' ' + esc(x.unit || '') : '') + (x.category ? ' · ' + esc(x.category) : '') + '</div></div><button class="rec-del" data-del-shop="' + x.id + '">×</button></div>'; }).join('');
      list.querySelectorAll('[data-shop]').forEach(function (b) { b.onclick = function () { var item = arr.filter(function (x) { return x.id === +b.getAttribute('data-shop'); })[0]; api('/api/shopping/' + item.id, { method: 'PATCH', body: { name: item.name, category: item.category, qty: item.qty, unit: item.unit, note: item.note, checked: !item.checked } }).then(shoppingView); }; });
      list.querySelectorAll('[data-del-shop]').forEach(function (b) { b.onclick = function () { api('/api/shopping/' + b.getAttribute('data-del-shop'), { method: 'DELETE' }).then(shoppingView); }; });
    }).catch(function (e) { $('shopList').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }
  function addShopSheet() {
    F.openSheet('添加采购项', '<div class="field"><span class="field-lb">内容</span><input id="shopName" class="field-in" placeholder="如 奶粉"></div><div class="row2"><div class="field half"><span class="field-lb">数量</span><input id="shopQty" class="field-in" inputmode="decimal"></div><div class="field half"><span class="field-lb">单位</span><input id="shopUnit" class="field-in" placeholder="罐 / 包"></div></div><div class="field"><span class="field-lb">分类</span><input id="shopCat" class="field-in" placeholder="消耗品"></div><button class="save" id="shopSave">加入清单</button>');
    $('shopSave').onclick = function () { api('/api/shopping', { method: 'POST', body: { name: $('shopName').value, qty: parseFloat($('shopQty').value) || 0, unit: $('shopUnit').value, category: $('shopCat').value } }).then(function () { F.closeSheet(); shoppingView(); toast('已加入采购清单'); }).catch(function (e) { toast(e.message); }); };
  }

  function relationsView() {
    content().innerHTML = header('关系温度', null, null) + '<div class="family-note">根据最近往来、欠礼和感谢状态，帮你找到该联系的人。</div><div id="relationList"><div class="skel" style="height:120px"></div></div>';
    api('/api/relationships?limit=100').then(function (j) {
      var arr = j.relationships || [], list = $('relationList');
      if (!arr.length) { list.innerHTML = '<div class="empty">还没有联系人</div>'; return; }
      list.innerHTML = arr.map(function (x) { var tone = x.temperature === '热络' ? 'hot' : (x.temperature === '沉寂' ? 'cold' : 'warm'); return '<div class="relation-row"><span class="temp ' + tone + '">' + esc(x.temperature) + '</span><div class="task-body"><div class="task-title">' + esc(x.name) + (x.relation ? ' <span class="rec-sub">' + esc(x.relation) + '</span>' : '') + '</div><div class="rec-sub">' + (x.days_since >= 0 ? x.days_since + ' 天未往来' : '暂无往来记录') + (x.pending_thanks ? ' · ' + x.pending_thanks + ' 笔待感谢' : '') + '</div></div>' + (x.suggested_cents ? '<span class="suggest">建议回礼 ¥' + fmt(x.suggested_cents) + '</span>' : '') + '</div>'; }).join('');
    }).catch(function (e) { $('relationList').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }

  function reportView() {
    var year = new Date().getFullYear();
    content().innerHTML = header('年度家庭报告', 'loadReport', '生成报告') + '<div class="field"><span class="field-lb">年份</span><input id="reportYear" class="field-in" inputmode="numeric" value="' + year + '"></div><div id="reportBox"><div class="empty">选择年份后生成家庭账单、成长节点和往来总结</div></div>';
    $('loadReport').onclick = loadReport;
    loadReport();
  }
  function loadReport() {
    var y = $('reportYear') ? $('reportYear').value : new Date().getFullYear();
    api(F.babyQuery('/api/report?year=' + encodeURIComponent(y))).then(function (j) {
      var r = j.report, html = '<div class="stat-grid"><div class="stat"><div class="stat-lb">收到礼金</div><div class="stat-v in">¥' + fmt(r.gift_recv) + '</div><div class="stat-x">' + r.gift_count + ' 笔 · ' + r.people + ' 人</div></div><div class="stat"><div class="stat-lb">宝宝花销</div><div class="stat-v warn">¥' + fmt(r.baby_total) + '</div><div class="stat-x">' + r.baby_count + ' 笔</div></div><div class="stat wide"><div class="stat-lb">送出礼金</div><div class="stat-v out">¥' + fmt(r.gift_sent) + '</div></div></div>';
      if (r.categories && r.categories.length) html += '<div class="card"><div class="card-h"><span class="card-t">花销分类</span></div>' + r.categories.map(function (x) { return '<div class="bar-row"><span class="bar-lb">' + esc(x.category) + '</span><span class="bar-bg"><span class="bar-fl" style="width:' + Math.min(100, Math.round(x.amount_cents / Math.max(1, r.baby_total) * 100)) + '%"></span></span><span class="bar-v">¥' + fmt(x.amount_cents) + '</span></div>'; }).join('') + '</div>';
      html += '<div class="card"><div class="card-h"><span class="card-t">成长节点</span></div>' + (r.milestones && r.milestones.length ? r.milestones.map(function (m) { return '<div class="rec"><span class="rec-dt">' + esc(m.occurred_on) + '</span><div class="rec-mid"><div class="rec-nm">' + esc(m.title) + '</div><div class="rec-sub">' + esc(m.note || m.kind) + '</div></div></div>'; }).join('') : '<div class="empty">这一年还没有记录成长节点</div>') + '</div>';
      html += '<div class="family-actions"><button class="btn btn-primary" id="printReport">打印 / 保存为 PDF</button><button class="btn" id="importCsv">导入 CSV</button></div>';
      $('reportBox').innerHTML = html;
      $('printReport').onclick = function () { window.print(); };
      $('importCsv').onclick = importCSVSheet;
    }).catch(function (e) { $('reportBox').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }
  function importCSVSheet() {
    F.openSheet('导入 CSV', '<div class="family-note">支持本应用导出的 CSV。人情账本列顺序：日期、姓名、关系、方向、金额、事由、方式、备注；宝宝开销列顺序：日期、分类、金额、数量、单位、商家、备注。</div><div class="cat-grid" id="csvBook"><button class="cat on" data-v="gift">人情账本</button><button class="cat" data-v="baby">宝宝开销</button></div><input id="csvFile" type="file" accept=".csv,text/csv" style="width:100%;padding:12px;background:var(--card);border:1px solid var(--line);border-radius:12px;color:var(--dim)"><button class="save" id="csvGo">开始导入</button>');
    var book = 'gift';
    $('csvBook').querySelectorAll('.cat').forEach(function (b) { b.onclick = function () { book = b.getAttribute('data-v'); $('csvBook').querySelectorAll('.cat').forEach(function (x) { x.classList.toggle('on', x === b); }); }; });
    $('csvGo').onclick = function () { var f = $('csvFile').files && $('csvFile').files[0]; if (!f) { toast('先选择 CSV 文件'); return; } var fr = new FileReader(); fr.onload = function () { api('/api/import/csv', { method: 'POST', body: { book: book, csv: String(fr.result).replace(/^\uFEFF/, '') } }).then(function (r) { F.closeSheet(); toast('已导入 ' + r.count + ' 条'); family(); }).catch(function (e) { toast(e.message); }); }; fr.readAsText(f, 'UTF-8'); };
  }

  function auditView() {
    content().innerHTML = header('家庭操作记录', null, null) + '<div id="auditList"><div class="skel" style="height:120px"></div></div>';
    api('/api/audit?limit=100').then(function (j) { var arr = j.logs || []; $('auditList').innerHTML = arr.length ? arr.map(function (x) { return '<div class="audit-row"><span class="audit-dot"></span><div class="task-body"><div class="task-title">' + esc(x.member || '家庭成员') + ' ' + esc(x.action) + ' 了 ' + esc(x.entity) + '</div><div class="rec-sub">' + esc(x.created_at) + (x.detail ? ' · ' + esc(x.detail) : '') + '</div></div></div>'; }).join('') : '<div class="empty">还没有操作记录</div>'; }).catch(function (e) { $('auditList').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; });
  }

  F.registerViews({ family: family });
})();
