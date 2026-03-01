// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later
//
// expr-builder.js — Reusable expression filter builder widget for rui.
// Usage:
//   var eb = new RuiExprBuilder({ container: 'my-container-id', outputId: 'my-hidden-input-id' });
//   eb.init();
// When the user clicks Apply, the built expression is written to the output element
// and the optional onApply() callback is invoked.

(function (global) {
  'use strict';

  // ── Unit and field definitions ──────────────────────────────────────────────
  var _speedUnits    = [{ label: 'B/s', f: 1 }, { label: 'KB/s', f: 1024 }, { label: 'MB/s', f: 1048576, dflt: true }, { label: 'GB/s', f: 1073741824 }];
  var _bytesUnits    = [{ label: 'B', f: 1 }, { label: 'KB', f: 1024 }, { label: 'MB', f: 1048576, dflt: true }, { label: 'GB', f: 1073741824 }, { label: 'TB', f: 1099511627776 }];
  var _durationUnits = [{ label: 'sec', f: 1 }, { label: 'min', f: 60, dflt: true }, { label: 'hr', f: 3600 }, { label: 'day', f: 86400 }, { label: 'week', f: 604800 }];
  var _timeAgoUnits  = [{ label: 's ago', ta: 1 }, { label: 'h ago', ta: 3600, dflt: true }, { label: 'd ago', ta: 86400 }, { label: 'w ago', ta: 604800 }, { label: 'mo ago', ta: 2592000 }];

  var DEFAULT_FIELDS = [
    { id: 'Name',          label: 'Name',           type: 'text' },
    { id: 'State',         label: 'State',          type: 'enum', values: ['downloading', 'uploading', 'stalledDL', 'stalledUP', 'pausedDL', 'pausedUP', 'stoppedDL', 'stoppedUP', 'error', 'missingFiles', 'checkingUP', 'checkingDL', 'forcedUP', 'forcedDL', 'metaDL', 'allocating', 'moving', 'queuedDL', 'queuedUP'] },
    { id: 'Category',      label: 'Category',       type: 'text' },
    { id: 'Tags',          label: 'Tags',           type: 'text' },
    { id: 'SavePath',      label: 'Save Path',      type: 'text' },
    { id: 'Tracker',       label: 'Tracker',        type: 'text' },
    { id: 'Ratio',         label: 'Ratio',          type: 'number' },
    { id: 'Progress',      label: 'Progress (0–1)', type: 'number' },
    { id: 'Priority',      label: 'Priority',       type: 'number' },
    { id: 'Availability',  label: 'Availability',   type: 'number' },
    { id: 'NumSeeds',      label: 'Seeds',          type: 'number' },
    { id: 'NumLeechs',     label: 'Leechers',       type: 'number' },
    { id: 'NumComplete',   label: 'Total Seeds',    type: 'number' },
    { id: 'NumIncomplete', label: 'Total Peers',    type: 'number' },
    { id: 'DlSpeed',       label: 'DL Speed',       type: 'number', units: _speedUnits },
    { id: 'UpSpeed',       label: 'UP Speed',       type: 'number', units: _speedUnits },
    { id: 'Size',          label: 'Size',           type: 'number', units: _bytesUnits },
    { id: 'TotalSize',     label: 'Total Size',     type: 'number', units: _bytesUnits },
    { id: 'Uploaded',      label: 'Uploaded',       type: 'number', units: _bytesUnits },
    { id: 'Downloaded',    label: 'Downloaded',     type: 'number', units: _bytesUnits },
    { id: 'AmountLeft',    label: 'Remaining',      type: 'number', units: _bytesUnits },
    { id: 'SeedingTime',   label: 'Seeding Time',   type: 'number', units: _durationUnits },
    { id: 'TimeActive',    label: 'Time Active',    type: 'number', units: _durationUnits },
    { id: 'AddedOn',       label: 'Added On',       type: 'number', units: _timeAgoUnits },
  ];

  var OPS_TEXT   = ['contains', '==', '!=', 'startsWith', 'endsWith'];
  var OPS_NUMBER = ['<', '<=', '>', '>=', '==', '!='];
  var OPS_ENUM   = ['==', '!='];

  var STATE_LABELS = {
    'downloading': 'Downloading', 'uploading': 'Seeding',
    'stalledDL': 'Stalled↓', 'stalledUP': 'Stalled↑',
    'pausedDL': 'Paused↓', 'pausedUP': 'Paused↑',
    'stoppedDL': 'Stopped↓', 'stoppedUP': 'Stopped↑',
    'error': 'Error', 'missingFiles': 'Missing Files',
    'checkingUP': 'Checking', 'checkingDL': 'Checking',
    'forcedUP': 'Forced UP', 'forcedDL': 'Forced DL',
    'metaDL': 'Getting Metadata', 'allocating': 'Allocating',
    'moving': 'Moving', 'queuedDL': 'Queued↓', 'queuedUP': 'Queued↑',
  };

  function _escHtml(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  // ── ExprBuilder constructor ──────────────────────────────────────────────────
  function ExprBuilder(cfg) {
    this._pfx       = cfg.prefix     || ('rxb' + Math.random().toString(36).slice(2, 6));
    this._container = typeof cfg.container === 'string' ? document.getElementById(cfg.container) : cfg.container;
    this._outputEl  = typeof cfg.outputEl  === 'string' ? document.getElementById(cfg.outputEl)  : cfg.outputEl;
    this._fields    = cfg.fields  || DEFAULT_FIELDS;
    this._onApply   = cfg.onApply || function () {};
    this._conds     = [];
    this._join      = 'and';
    this._rangeOpen = false;
    this._editPath  = null;
    this._sortable  = null;
  }

  // ── Internal helpers ──────────────────────────────────────────────────────────
  ExprBuilder.prototype._id = function (suffix) { return this._pfx + '-' + suffix; };
  ExprBuilder.prototype._el = function (suffix) { return document.getElementById(this._id(suffix)); };

  ExprBuilder.prototype._getField = function (id) {
    return this._fields.find(function (f) { return f.id === id; });
  };

  ExprBuilder.prototype._getUnit = function (field, idx) {
    return (field && field.units && idx >= 0) ? (field.units[idx] || null) : null;
  };

  ExprBuilder.prototype._computeVal = function (rawVal, field, unitIdx) {
    var v = parseFloat(rawVal);
    if (isNaN(v)) return rawVal;
    var u = this._getUnit(field, unitIdx);
    if (!u) return v;
    if (u.ta) return Math.floor(Date.now() / 1000) - v * u.ta;
    return (u.f && u.f !== 1) ? Math.round(v * u.f) : v;
  };

  ExprBuilder.prototype._buildAtom = function (field, op, computedVal) {
    var s = String(computedVal);
    if (op === 'contains')   return field.id + ' contains "' + s + '"';
    if (op === 'startsWith') return field.id + ' startsWith "' + s + '"';
    if (op === 'endsWith')   return field.id + ' endsWith "' + s + '"';
    if (field.type === 'text' || field.type === 'enum') return field.id + ' ' + op + ' "' + s + '"';
    return field.id + ' ' + op + ' ' + s;
  };

  ExprBuilder.prototype._condToExpr = function (c) {
    var self = this;
    if (c.kind === 'group') {
      var inner = c.conditions.map(function (cc, i) {
        return (i === 0 ? '' : (cc.join === 'or' ? ' || ' : ' && ')) + self._condToExpr(cc);
      }).join('');
      return '(' + inner + ')';
    }
    var field = this._getField(c.fieldId);
    if (!field) return '';
    var v1 = this._computeVal(c.rawVal, field, c.unitIdx);
    var e1 = this._buildAtom(field, c.op, v1);
    if (c.rangeOp && c.rawVal2 !== '') {
      var uIdx2 = c.unitIdx2 >= 0 ? c.unitIdx2 : c.unitIdx;
      var v2 = this._computeVal(c.rawVal2, field, uIdx2);
      return '(' + e1 + ' && ' + this._buildAtom(field, c.rangeOp, v2) + ')';
    }
    return e1;
  };

  ExprBuilder.prototype._condLabel = function (c) {
    if (c.kind === 'group') return '( ' + c.conditions.length + ' conditions )';
    var field = this._getField(c.fieldId);
    if (!field) return c.rawVal || '';
    var u1  = this._getUnit(field, c.unitIdx);
    var lbl = field.label + ' ' + c.op + ' ' + c.rawVal + (u1 ? '\u202f' + u1.label : '');
    if (c.rangeOp && c.rawVal2 !== '') {
      var uIdx2 = c.unitIdx2 >= 0 ? c.unitIdx2 : c.unitIdx;
      var u2 = this._getUnit(field, uIdx2);
      lbl += ' and ' + c.rangeOp + ' ' + c.rawVal2 + (u2 ? '\u202f' + u2.label : '');
    }
    return lbl;
  };

  // ── Render input widget for a field / unit pair ─────────────────────────────
  ExprBuilder.prototype._valInput = function (valId, unitId, field) {
    var self   = this;
    var fId    = this._id(valId);
    var uId    = this._id(unitId);
    var cls    = 'h-7 rounded-md border border-input bg-background px-2 text-xs focus:outline-none focus:ring-1 focus:ring-ring';
    var enter  = 'onkeydown="if(event.key===\'Enter\'){event.preventDefault();' + self._pfx + '_stage();}"';
    if (field.type === 'enum') {
      return '<select id="' + fId + '" class="' + cls + '">' +
        (field.values || []).map(function (v) {
          return '<option value="' + v + '">' + (_escHtml(STATE_LABELS[v] || v)) + '</option>';
        }).join('') + '</select>';
    }
    var hasUnits = field.type === 'number' && field.units && field.units.length;
    var wCls    = hasUnits ? 'w-16' : 'w-24';
    var tAttr   = field.type === 'number' ? 'type="number" step="any"' : 'type="text"';
    var html    = '<input id="' + fId + '" ' + tAttr + ' placeholder="value" ' + enter + ' class="' + cls + ' ' + wCls + '"/>';
    if (hasUnits) {
      html += '<select id="' + uId + '" class="' + cls + ' ml-1">';
      field.units.forEach(function (u, i) {
        html += '<option value="' + i + '"' + (u.dflt ? ' selected' : '') + '>' + u.label + '</option>';
      });
      html += '</select>';
    }
    return html;
  };

  // ── Render the full builder UI inside this._container ───────────────────────
  ExprBuilder.prototype.init = function () {
    var self = this;
    if (!this._container) { console.warn('RuiExprBuilder: container not found'); return; }

    // Expose stage/fieldChanged to global scope (prefixed) so inline onclick will work.
    global[this._pfx + '_stage']        = function () { self._stageAdd(); };
    global[this._pfx + '_fieldChanged'] = function () { self._fieldChanged(); };
    global[this._pfx + '_toggleRange']  = function () { self._toggleRange(); };
    global[this._pfx + '_setJoin']      = function (j) { self._setJoin(j); };
    global[this._pfx + '_toggleCheck']  = function (ri) { self._toggleCheck(ri); };
    global[this._pfx + '_removeCond']   = function (ri, ii) { self._removeCond(ri, ii); };
    global[this._pfx + '_editCond']     = function (ri, ii) { self._editCond(ri, ii); };
    global[this._pfx + '_cancelEdit']   = function () { self._cancelEdit(); };
    global[this._pfx + '_group']        = function () { self._groupChecked(); };
    global[this._pfx + '_ungroup']      = function (ri) { self._ungroupAt(ri); };
    global[this._pfx + '_clearAll']     = function () { self._clearAll(); };
    global[this._pfx + '_commit']       = function () { self._commit(); };
    global[this._pfx + '_toggleJoin']   = function (ri, ii, j) { self._toggleJoin(ri, ii, j); };

    var p = this._pfx;
    var fieldOpts = this._fields.map(function (f) {
      return '<option value="' + f.id + '">' + _escHtml(f.label) + '</option>';
    }).join('');

    var html = [
      '<p class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Add Condition</p>',
      // Row 1: field + op + value + range toggle
      '<div class="flex items-center gap-1.5 flex-wrap">',
      '  <select id="' + this._id('field') + '" onchange="' + p + '_fieldChanged()" class="h-7 rounded-md border border-input bg-background px-2 text-xs focus:outline-none focus:ring-1 focus:ring-ring">' + fieldOpts + '</select>',
      '  <select id="' + this._id('op') + '" class="h-7 rounded-md border border-input bg-background px-2 text-xs focus:outline-none focus:ring-1 focus:ring-ring"></select>',
      '  <div id="' + this._id('val-cont') + '"></div>',
      '  <button type="button" id="' + this._id('range-btn') + '" onclick="' + p + '_toggleRange()" title="Add second bound (range)" class="h-7 px-2 rounded-md border border-input bg-background text-xs text-muted-foreground hover:bg-accent hover:text-foreground transition-colors">⇔</button>',
      '</div>',
      // Row 2: second bound (hidden by default)
      '<div id="' + this._id('range-row') + '" class="hidden flex items-center gap-1.5 mt-1">',
      '  <span class="text-[10px] text-muted-foreground font-medium uppercase tracking-wider">and</span>',
      '  <select id="' + this._id('op2') + '" class="h-7 rounded-md border border-input bg-background px-2 text-xs focus:outline-none focus:ring-1 focus:ring-ring"></select>',
      '  <div id="' + this._id('val2-cont') + '"></div>',
      '</div>',
      // Row 3: connector toggle + add button
      '<div class="flex items-center justify-between mt-1">',
      '  <div class="inline-flex rounded border border-border overflow-hidden text-[10px] font-semibold" title="Connector">',
      '    <button id="' + this._id('join-and') + '" type="button" onclick="' + p + '_setJoin(\'and\')" class="px-2 py-0.5 bg-primary text-primary-foreground">AND</button>',
      '    <button id="' + this._id('join-or') + '"  type="button" onclick="' + p + '_setJoin(\'or\')"  class="px-2 py-0.5 bg-background text-muted-foreground hover:bg-accent hover:text-foreground transition-colors">OR</button>',
      '  </div>',
      '  <div class="flex items-center gap-2">',
      '    <span id="' + this._id('cancel-edit') + '" class="hidden text-xs text-muted-foreground cursor-pointer hover:text-foreground transition-colors" onclick="' + p + '_cancelEdit()">Cancel</span>',
      '    <button id="' + this._id('add-btn') + '" type="button" onclick="' + p + '_stage()" class="h-7 rounded-md bg-primary px-2.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 transition-colors">+ Add</button>',
      '  </div>',
      '</div>',
      // Staged conditions list
      '<ol id="' + this._id('conds') + '" class="space-y-1 list-none max-h-48 overflow-y-auto mt-1"></ol>',
      // Footer (apply / clear / group)
      '<div id="' + this._id('footer') + '" class="hidden border-t border-border pt-2 flex items-center justify-between gap-2 mt-1">',
      '  <div class="flex items-center gap-2">',
      '    <button id="' + this._id('group-btn') + '" type="button" onclick="' + p + '_group()" class="hidden h-6 rounded border border-border px-2 text-[10px] font-medium text-muted-foreground hover:bg-accent hover:text-foreground transition-colors">↱ Group checked</button>',
      '    <button type="button" onclick="' + p + '_clearAll()" class="text-xs text-muted-foreground hover:text-foreground transition-colors">Clear all</button>',
      '  </div>',
      '  <button type="button" onclick="' + p + '_commit()" class="h-7 rounded-md bg-primary px-3 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors">Apply</button>',
      '</div>',
    ].join('\n');

    this._container.innerHTML = html;
    this._fieldChanged();

    // Populate output field with current value if set from a previous session.
    if (this._outputEl && this._outputEl.value) {
      this._parseExprToStaged(this._outputEl.value);
    }
  };

  // ── Populate op select + val input after field change ───────────────────────
  ExprBuilder.prototype._fieldChanged = function () {
    var fieldSel = this._el('field');
    if (!fieldSel) return;
    var field = this._getField(fieldSel.value);
    if (!field) return;
    var ops = field.type === 'number' ? OPS_NUMBER : field.type === 'enum' ? OPS_ENUM : OPS_TEXT;
    var opOpts = ops.map(function (o) { return '<option value="' + o + '">' + o + '</option>'; }).join('');
    var opSel = this._el('op'), opSel2 = this._el('op2');
    var valC  = this._el('val-cont'), val2C = this._el('val2-cont');
    if (opSel)  opSel.innerHTML  = opOpts;
    if (opSel2) opSel2.innerHTML = opOpts;
    if (valC)   valC.innerHTML   = this._valInput('val',  'unit',  field);
    if (val2C)  val2C.innerHTML  = this._valInput('val2', 'unit2', field);
  };

  // ── Toggle range row ─────────────────────────────────────────────────────────
  ExprBuilder.prototype._toggleRange = function () {
    this._rangeOpen = !this._rangeOpen;
    var row = this._el('range-row'), btn = this._el('range-btn');
    if (row) row.classList.toggle('hidden', !this._rangeOpen);
    if (btn) {
      btn.classList.toggle('bg-accent',           this._rangeOpen);
      btn.classList.toggle('text-foreground',     this._rangeOpen);
      btn.classList.toggle('text-muted-foreground', !this._rangeOpen);
    }
  };

  // ── Set join connector ───────────────────────────────────────────────────────
  ExprBuilder.prototype._setJoin = function (j) {
    this._join = j;
    var act = 'px-2 py-0.5 bg-primary text-primary-foreground';
    var off = 'px-2 py-0.5 bg-background text-muted-foreground hover:bg-accent hover:text-foreground transition-colors';
    var a = this._el('join-and'), o = this._el('join-or');
    if (a) a.className = j === 'and' ? act : off;
    if (o) o.className = j === 'or'  ? act : off;
  };

  // ── Read inputs → build a cond object ───────────────────────────────────────
  ExprBuilder.prototype._readInputs = function () {
    var fieldSel = this._el('field'), opSel = this._el('op'), valEl = this._el('val'), unitSel = this._el('unit');
    if (!fieldSel || !opSel || !valEl) return null;
    var field = this._getField(fieldSel.value);
    if (!field) return null;
    var val = valEl.value.trim();
    if (val === '') return null;
    var unitIdx = unitSel ? parseInt(unitSel.value, 10) : -1;
    var rangeOp = null, rawVal2 = '', unitIdx2 = -1;
    if (this._rangeOpen) {
      var op2Sel = this._el('op2'), val2El = this._el('val2'), unit2Sel = this._el('unit2');
      if (op2Sel && val2El && val2El.value.trim() !== '') {
        rangeOp  = op2Sel.value;
        rawVal2  = val2El.value.trim();
        unitIdx2 = unit2Sel ? parseInt(unit2Sel.value, 10) : unitIdx;
      }
    }
    return { kind: 'cond', join: this._join, checked: false,
      fieldId: fieldSel.value, op: opSel.value,
      rawVal: val, unitIdx: unitIdx,
      rangeOp: rangeOp, rawVal2: rawVal2, unitIdx2: unitIdx2 };
  };

  // ── Stage: add / update ──────────────────────────────────────────────────────
  ExprBuilder.prototype._stageAdd = function () {
    var c = this._readInputs();
    if (!c) return;
    if (this._editPath !== null) {
      var ep = this._editPath;
      var target = ep.innerIdx !== null
        ? this._conds[ep.rootIdx].conditions[ep.innerIdx]
        : this._conds[ep.rootIdx];
      c.join = target.join;
      if (ep.innerIdx !== null) this._conds[ep.rootIdx].conditions[ep.innerIdx] = c;
      else this._conds[ep.rootIdx] = c;
      this._editPath = null;
      var addBtn = this._el('add-btn');   if (addBtn)   addBtn.textContent = '+ Add';
      var cancel  = this._el('cancel-edit'); if (cancel) cancel.classList.add('hidden');
    } else {
      this._conds.push(c);
    }
    var valEl = this._el('val'); if (valEl) valEl.value = '';
    var v2    = this._el('val2'); if (v2) v2.value = '';
    this._renderConditions();
  };

  // ── Edit a staged condition ──────────────────────────────────────────────────
  ExprBuilder.prototype._editCond = function (rootIdx, innerIdx) {
    var c = innerIdx !== null
      ? this._conds[rootIdx].conditions[innerIdx]
      : this._conds[rootIdx];
    if (!c || c.kind !== 'cond') return;
    this._editPath = { rootIdx: rootIdx, innerIdx: innerIdx };
    var fieldSel = this._el('field');
    if (fieldSel) { fieldSel.value = c.fieldId; this._fieldChanged(); }
    var opSel = this._el('op');  if (opSel) opSel.value  = c.op;
    var valEl = this._el('val'); if (valEl) valEl.value = c.rawVal;
    var uSel  = this._el('unit'); if (uSel && c.unitIdx >= 0) uSel.value = c.unitIdx;
    if (c.rangeOp) {
      if (!this._rangeOpen) this._toggleRange();
      var op2 = this._el('op2'); if (op2) op2.value = c.rangeOp;
      var v2  = this._el('val2'); if (v2) v2.value = c.rawVal2;
      var u2  = this._el('unit2'); if (u2 && c.unitIdx2 >= 0) u2.value = c.unitIdx2;
    } else {
      if (this._rangeOpen) this._toggleRange();
    }
    var addBtn = this._el('add-btn');    if (addBtn)  addBtn.textContent = '\u2713 Update';
    var cancel  = this._el('cancel-edit'); if (cancel) cancel.classList.remove('hidden');
  };

  ExprBuilder.prototype._cancelEdit = function () {
    this._editPath = null;
    var addBtn = this._el('add-btn');    if (addBtn)  addBtn.textContent = '+ Add';
    var cancel  = this._el('cancel-edit'); if (cancel) cancel.classList.add('hidden');
  };

  // ── Toggle join on an existing staged condition ──────────────────────────────
  ExprBuilder.prototype._toggleJoin = function (rootIdx, innerIdx, j) {
    var t = (innerIdx !== null && this._conds[rootIdx])
      ? this._conds[rootIdx].conditions[innerIdx]
      : this._conds[rootIdx];
    if (t) { t.join = j; this._renderConditions(); }
  };

  ExprBuilder.prototype._removeCond = function (rootIdx, innerIdx) {
    if (innerIdx !== null) {
      this._conds[rootIdx].conditions.splice(innerIdx, 1);
      if (this._conds[rootIdx].conditions.length === 0) this._conds.splice(rootIdx, 1);
    } else {
      this._conds.splice(rootIdx, 1);
    }
    this._renderConditions();
  };

  ExprBuilder.prototype._toggleCheck = function (rootIdx) {
    if (this._conds[rootIdx]) { this._conds[rootIdx].checked = !this._conds[rootIdx].checked; this._renderConditions(); }
  };

  ExprBuilder.prototype._groupChecked = function () {
    var idxs = [];
    this._conds.forEach(function (c, i) { if (c.checked) idxs.push(i); });
    if (idxs.length < 2) return;
    var kids = idxs.map(function (i) { return Object.assign({}, this._conds[i], { checked: false }); }, this);
    var groupJoin = kids[0].join; kids[0].join = 'and';
    var group = { kind: 'group', join: groupJoin, checked: false, conditions: kids };
    var at = idxs[0];
    for (var i = idxs.length - 1; i >= 0; i--) this._conds.splice(idxs[i], 1);
    this._conds.splice(at, 0, group);
    this._renderConditions();
  };

  ExprBuilder.prototype._ungroupAt = function (rootIdx) {
    var g = this._conds[rootIdx];
    if (!g || g.kind !== 'group') return;
    var kids = g.conditions.map(function (c) { return Object.assign({}, c, { checked: false }); });
    if (kids.length) kids[0].join = g.join;
    [].splice.apply(this._conds, [rootIdx, 1].concat(kids));
    this._renderConditions();
  };

  ExprBuilder.prototype._clearAll = function () {
    this._conds = [];
    this._cancelEdit();
    this._renderConditions();
  };

  // ── Render the staged conditions list ───────────────────────────────────────
  ExprBuilder.prototype._renderConditions = function () {
    var self     = this;
    var list     = this._el('conds');
    var footer   = this._el('footer');
    var groupBtn = this._el('group-btn');
    if (!list) return;

    var checkedCount = this._conds.filter(function (c) { return c.checked; }).length;
    if (groupBtn) groupBtn.classList.toggle('hidden', checkedCount < 2);
    if (footer)   footer.classList.toggle('hidden', this._conds.length === 0);

    var p   = this._pfx;
    var JAC = 'inline-flex h-5 px-1.5 rounded-l text-[10px] font-semibold cursor-pointer select-none transition-colors';
    var JOC = 'inline-flex h-5 px-1.5 rounded-r text-[10px] font-semibold cursor-pointer select-none transition-colors';

    function joinPill(item, ri, ii) {
      var isOr = item.join === 'or';
      var iiArg = ii !== null ? ',' + ii : ',null';
      return '<div class="inline-flex rounded border border-border overflow-hidden shrink-0">'
        + '<button type="button" class="' + JAC + (isOr ? ' bg-muted text-muted-foreground' : ' bg-primary text-primary-foreground') + '" onclick="' + p + '_toggleJoin(' + ri + iiArg + ',\'and\')">AND</button>'
        + '<button type="button" class="' + JOC + (!isOr ? ' bg-muted text-muted-foreground' : ' bg-primary text-primary-foreground') + '" onclick="' + p + '_toggleJoin(' + ri + iiArg + ',\'or\')">OR</button>'
        + '</div>';
    }

    function condRow(c, ri, ii) {
      var inGroup  = ii !== null;
      var showJoin = inGroup ? ii > 0 : ri > 0;
      var join = showJoin ? joinPill(c, ri, inGroup ? ii : null) : '';
      var iiArg = ii !== null ? ii : 'null';
      var lbl = _escHtml(self._condLabel(c));
      return '<li class="eb-cond-row flex items-center gap-1 rounded bg-muted/40 px-1.5 py-0.5' + (inGroup ? ' ml-2' : '') + '">'
        + '<span class="cursor-grab text-muted-foreground shrink-0">\u2807</span>'
        + (!inGroup ? '<input type="checkbox" class="shrink-0 rounded"' + (c.checked ? ' checked' : '') + ' onchange="' + p + '_toggleCheck(' + ri + ')" />' : '')
        + join
        + '<code class="flex-1 text-xs font-mono truncate min-w-0 mx-0.5" title="' + lbl + '">' + lbl + '</code>'
        + '<button type="button" onclick="' + p + '_editCond(' + ri + ',' + iiArg + ')" class="shrink-0 text-muted-foreground hover:text-foreground transition-colors text-xs" title="Edit">\u270e</button>'
        + '<button type="button" onclick="' + p + '_removeCond(' + ri + ',' + iiArg + ')" class="shrink-0 text-muted-foreground hover:text-destructive transition-colors text-sm leading-none" title="Remove">\u00d7</button>'
        + '</li>';
    }

    list.innerHTML = this._conds.map(function (c, i) {
      if (c.kind === 'group') {
        var gJoin = i > 0 ? joinPill(c, i, null) : '';
        var inner = c.conditions.map(function (cc, j) { return condRow(cc, i, j); }).join('');
        return '<li class="eb-cond-row list-none rounded border border-border/60 bg-muted/20 p-1" data-root="' + i + '">'
          + '<div class="flex items-center gap-1 mb-0.5">'
          + '<span class="cursor-grab text-muted-foreground shrink-0">\u2807</span>'
          + '<input type="checkbox" class="shrink-0 rounded"' + (c.checked ? ' checked' : '') + ' onchange="' + p + '_toggleCheck(' + i + ')" />'
          + gJoin
          + '<span class="text-[10px] text-muted-foreground flex-1 ml-0.5">Group (' + c.conditions.length + ')</span>'
          + '<button type="button" onclick="' + p + '_ungroup(' + i + ')" class="text-[10px] text-muted-foreground hover:text-foreground transition-colors" title="Ungroup">\u21b1 ungroup</button>'
          + '<button type="button" onclick="' + p + '_removeCond(' + i + ',null)" class="shrink-0 text-muted-foreground hover:text-destructive transition-colors text-sm leading-none ml-1">\u00d7</button>'
          + '</div>'
          + '<ol class="eb-group-inner space-y-0.5 list-none" data-gidx="' + i + '">' + inner + '</ol>'
          + '</li>';
      }
      return condRow(c, i, null);
    }).join('');

    // Set up Sortable on root list.
    if (this._sortable) { this._sortable.destroy(); this._sortable = null; }
    if (this._conds.length > 1 && typeof Sortable !== 'undefined') {
      var self2 = this;
      this._sortable = Sortable.create(list, {
        animation: 120, handle: '.eb-cond-row', ghostClass: 'opacity-40',
        onEnd: function (evt) {
          if (evt.oldIndex === evt.newIndex) return;
          var moved = self2._conds.splice(evt.oldIndex, 1)[0];
          self2._conds.splice(evt.newIndex, 0, moved);
          self2._renderConditions();
        },
      });
    }
    // Per-group inner Sortables.
    var conds = this._conds;
    list.querySelectorAll('.eb-group-inner').forEach(function (el) {
      var gIdx = parseInt(el.dataset.gidx, 10);
      if (typeof Sortable === 'undefined' || !conds[gIdx] || conds[gIdx].conditions.length < 2) return;
      var grp = conds[gIdx].conditions;
      Sortable.create(el, {
        animation: 120, handle: '.eb-cond-row', ghostClass: 'opacity-40',
        onEnd: function (evt) {
          if (evt.oldIndex === evt.newIndex) return;
          var moved = grp.splice(evt.oldIndex, 1)[0];
          grp.splice(evt.newIndex, 0, moved);
          self._renderConditions();
        },
      });
    });
  };

  // ── Commit: serialize staged list → output field ─────────────────────────────
  ExprBuilder.prototype._commit = function () {
    var self = this;
    if (this._conds.length === 0) return;
    var built = this._conds.map(function (c, i) {
      return (i === 0 ? '' : (c.join === 'or' ? ' || ' : ' && ')) + self._condToExpr(c);
    }).join('');
    if (this._outputEl) {
      var prev = this._outputEl.value.trim();
      this._outputEl.value = (prev && prev !== built)
        ? '(' + prev + ') && (' + built + ')'
        : built;
    }
    this._conds = [];
    this._cancelEdit();
    this._renderConditions();
    this._onApply(built);
  };

  // ── Parse a simple flat expr string back into staged conditions ──────────────
  // This is a best-effort reverse parse; supports flat && chains.
  ExprBuilder.prototype._parseExprToStaged = function (exprStr) {
    // Leave complex / nested exprs in the output field only.
    // We don't try to reconstruct grouped/OR conditions from raw strings.
    // Simple "Field op Value && ..." chains can be pre-staged.
    // For now just leave existing value as-is and show it in a chip.
  };

  // ── Destroy / cleanup ────────────────────────────────────────────────────────
  ExprBuilder.prototype.destroy = function () {
    if (this._sortable) { this._sortable.destroy(); this._sortable = null; }
    var p = this._pfx;
    ['_stage', '_fieldChanged', '_toggleRange', '_setJoin', '_toggleCheck',
     '_removeCond', '_editCond', '_cancelEdit', '_group', '_ungroup', '_clearAll', '_commit', '_toggleJoin'].forEach(function (fn) {
      delete global[p + fn];
    });
    if (this._container) this._container.innerHTML = '';
  };

  // ── Public API ───────────────────────────────────────────────────────────────
  global.RuiExprBuilder = ExprBuilder;

})(window);
