/**
 * VirtualList — lightweight virtual-scroll engine for <tbody> elements.
 *
 * Only rows visible in the scroll viewport are inserted into the DOM.
 * Top and bottom spacer rows keep the scrollbar proportionally correct.
 * Row nodes are cached after first creation so they are not rebuilt on
 * every scroll event.
 *
 * Usage:
 *   var list = new VirtualList({
 *     containerId : 'my-scroll-wrapper',   // overflow-auto/scroll element
 *     tbodyId     : 'my-tbody',            // <tbody> or <ul> to populate
 *     colSpan     : 6,                     // columns for spacer <td> (table mode)
 *     rowTag      : 'tr',                  // 'tr' (default, table) or 'li' (list)
 *     renderRow   : function(item, i) {    // must return the element for rowTag
 *                     var tr = document.createElement('tr');
 *                     tr.textContent = item.name;
 *                     return tr;
 *                   },
 *     // optional
 *     rowHeight   : 40,         // initial row-height estimate (px, auto-measured)
 *     overscan    : 5,          // extra rows rendered beyond the viewport
 *     emptyText   : 'No items.', // shown when rows array is empty
 *     afterRender : function() { /* called after each DOM update *\/ },
 *   });
 *
 *   list.load(arrayOfItems);   // replace data and re-render
 *   list.render();             // force a layout pass (e.g. after resize)
 *   list.invalidate();         // clear node cache (nodes recreated next render)
 *   list.destroy();            // disconnect observers
 *
 *   list.rows                  // current data array (read access)
 */
(function (global) {
  'use strict';

  function VirtualList(opts) {
    this._cid         = opts.containerId;
    this._tbid        = opts.tbodyId;
    this._colSpan     = opts.colSpan     || 1;
    this._rowTag      = (opts.rowTag     || 'tr').toLowerCase();
    this._rh          = opts.rowHeight   || 40;
    this._overscan    = opts.overscan    || 5;
    this._renderRow   = opts.renderRow;
    this._emptyText   = opts.emptyText   || 'No items.';
    this._afterRender = opts.afterRender || null;
    this.rows         = [];
    this._cache       = {};
    this._listening   = false;
    this._topId       = opts.tbodyId + '-vl-top';
    this._botId       = opts.tbodyId + '-vl-bot';
    this._ro          = null;
  }

  VirtualList.prototype._el = function (id) {
    return document.getElementById(id);
  };

  VirtualList.prototype._mkSpacer = function (id) {
    if (this._rowTag === 'tr') {
      // Table mode: spacer is a <tr><td colspan=N> so total height is set on the td
      var tr = document.createElement('tr');
      tr.id  = id;
      var td = document.createElement('td');
      td.setAttribute('colspan', String(this._colSpan));
      td.style.padding = '0';
      td.style.height  = '0px';
      tr.appendChild(td);
      return tr;
    } else {
      // List mode: spacer is a plain <li> (or whatever rowTag) with height directly
      var el = document.createElement(this._rowTag);
      el.id             = id;
      el.style.padding  = '0';
      el.style.height   = '0px';
      el.style.overflow = 'hidden';
      el.style.listStyle= 'none';
      el.style.display  = 'block';
      return el;
    }
  };

  // _setSpacerH sets the height of a spacer produced by _mkSpacer.
  VirtualList.prototype._setSpacerH = function (spacer, px) {
    var target = this._rowTag === 'tr' ? spacer.firstChild : spacer;
    target.style.height = px + 'px';
  };

  // _emptyEl creates the "no items" row appropriate for the list mode.
  VirtualList.prototype._emptyEl = function () {
    if (this._rowTag === 'tr') {
      var tr = document.createElement('tr');
      tr.innerHTML = '<td colspan="' + this._colSpan + '" class="px-3 py-12 text-center text-sm text-muted-foreground">' +
        this._emptyText + '</td>';
      return tr;
    } else {
      var li = document.createElement(this._rowTag);
      li.className = 'px-2 py-3 text-center text-xs text-muted-foreground';
      li.textContent = this._emptyText;
      li.style.listStyle = 'none';
      return li;
    }
  };

  /**
   * Replace the data array and redraw.
   * @param {Array} rows
   */
  VirtualList.prototype.load = function (rows) {
    this.rows   = rows || [];
    this._cache = {};
    this._rh    = 40; // reset so height is re-measured

    var tbody     = this._el(this._tbid);
    var container = this._el(this._cid);
    if (!tbody || !container) return;

    // Remove stale spacers and clear the tbody
    var old;
    old = this._el(this._topId); if (old) old.parentNode.removeChild(old);
    old = this._el(this._botId); if (old) old.parentNode.removeChild(old);
    while (tbody.firstChild) tbody.removeChild(tbody.firstChild);
    tbody.appendChild(this._mkSpacer(this._topId));
    tbody.appendChild(this._mkSpacer(this._botId));

    // Attach scroll listener once per instance
    if (!this._listening) {
      var self = this;
      container.addEventListener('scroll', function () { self.render(); }, { passive: true });
      this._listening = true;
    }

    // Defer first render until the scroll container has a real measured height.
    // clientHeight is 0 during initial page load before layout completes.
    if (container.clientHeight > 0) {
      this.render();
    } else if (typeof ResizeObserver !== 'undefined') {
      if (this._ro) this._ro.disconnect();
      var self = this;
      this._ro = new ResizeObserver(function () {
        if (container.clientHeight > 0) {
          self._ro.disconnect();
          self._ro = null;
          self.render();
        }
      });
      this._ro.observe(container);
    } else {
      var self = this;
      setTimeout(function () { self.render(); }, 100);
    }
  };

  /** Force a layout pass — call after external size changes. */
  VirtualList.prototype.render = function () {
    var tbody     = this._el(this._tbid);
    var container = this._el(this._cid);
    var top       = this._el(this._topId);
    var bot       = this._el(this._botId);
    if (!tbody || !container || !top || !bot) return;

    var total = this.rows.length;
    var rh    = this._rh;

    // Remove current content rows (between the two spacers)
    var node = top.nextSibling;
    while (node && node !== bot) {
      var nxt = node.nextSibling;
      tbody.removeChild(node);
      node = nxt;
    }

    if (total === 0) {
      var emptyEl = this._emptyEl();
      tbody.insertBefore(emptyEl, bot);
      this._setSpacerH(top, 0);
      this._setSpacerH(bot, 0);
      if (this._afterRender) this._afterRender();
      return;
    }

    var scrollTop = container.scrollTop;
    var viewH     = container.clientHeight;
    var overscan  = this._overscan;
    var startIdx  = Math.max(0, Math.floor(scrollTop / rh) - overscan);
    var endIdx    = Math.min(total, Math.ceil((scrollTop + viewH) / rh) + overscan);

    this._setSpacerH(top, startIdx * rh);
    this._setSpacerH(bot, (total - endIdx) * rh);

    var cache     = this._cache;
    var renderRow = this._renderRow;
    var rows      = this.rows;
    var frag      = document.createDocumentFragment();
    for (var i = startIdx; i < endIdx; i++) {
      if (!cache[i]) cache[i] = renderRow(rows[i], i);
      frag.appendChild(cache[i]);
    }
    tbody.insertBefore(frag, bot);

    // Auto-measure actual row height after the first real render
    if (this._rh === 40) {
      var first = top.nextSibling;
      if (first && first !== bot) {
        var h = first.getBoundingClientRect().height;
        if (h > 4) this._rh = h;
      }
    }

    if (this._afterRender) this._afterRender();
  };

  /**
   * Clear the node cache. All rows will be recreated on the next render.
   * Useful when underlying item data changes in-place without a full load().
   */
  VirtualList.prototype.invalidate = function () {
    this._cache = {};
  };

  /** Disconnect observers. Safe to discard the instance after this. */
  VirtualList.prototype.destroy = function () {
    if (this._ro) { this._ro.disconnect(); this._ro = null; }
    this.rows   = [];
    this._cache = {};
  };

  global.VirtualList = VirtualList;
}(window));
