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
 *     overscan    : 15,         // extra rows rendered beyond the viewport
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
    this._initialRh   = this._rh;
    this._rhMeasured  = false;          // true once we've read an actual row height
    this._overscan    = opts.overscan    || 15;
    this._renderRow   = opts.renderRow;
    this._emptyText   = opts.emptyText   || 'No items.';
    this._afterRender = opts.afterRender || null;
    this.rows         = [];
    this._cache       = {};
    this._listening   = false;
    this._topId       = opts.tbodyId + '-vl-top';
    this._botId       = opts.tbodyId + '-vl-bot';
    this._ro          = null;
    this._raf         = null;
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
    // Do NOT reset _rh — preserve the measured row height across data refreshes
    // so render() immediately targets the correct viewport window.

    var tbody     = this._el(this._tbid);
    var container = this._el(this._cid);
    if (!tbody || !container) return;

    // Save scroll position before touching the DOM.
    // Removing all rows collapses scrollHeight to near-zero, causing the browser
    // to clip both scrollTop and scrollLeft back to 0.
    var savedScrollTop  = container.scrollTop;
    var savedScrollLeft = container.scrollLeft;

    // Remove stale spacers and clear the tbody
    var old;
    old = this._el(this._topId); if (old) old.parentNode.removeChild(old);
    old = this._el(this._botId); if (old) old.parentNode.removeChild(old);
    while (tbody.firstChild) tbody.removeChild(tbody.firstChild);

    var topSpacer = this._mkSpacer(this._topId);
    var botSpacer = this._mkSpacer(this._botId);
    tbody.appendChild(topSpacer);
    tbody.appendChild(botSpacer);

    // Pre-size the bottom spacer to the approximate full content height so that
    // scrollHeight is large enough for the restored position before render().
    this._setSpacerH(botSpacer, this.rows.length * this._rh);

    // Restore scroll position.
    container.scrollTop  = savedScrollTop;
    container.scrollLeft = savedScrollLeft;

    // Attach scroll + resize listeners once per instance
    if (!this._listening) {
      var self = this;
      container.addEventListener('scroll', function () {
        // Throttle to one render per animation frame — stays synchronous with
        // the paint cycle but avoids redundant layout work on rapid scroll.
        if (self._raf) return;
        self._raf = requestAnimationFrame(function () {
          self._raf = null;
          self.render();
        });
      }, { passive: true });
      window.addEventListener('resize', function () { self.render(); }, { passive: true });
      // Re-render when the scroll container itself is resized (e.g. detail panel open/close)
      if (typeof ResizeObserver !== 'undefined') {
        this._containerRO = new ResizeObserver(function () { self.render(); });
        this._containerRO.observe(container);
      }
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

    // Capture scroll state BEFORE any DOM mutations.  Reading layout
    // properties after removeChild forces a reflow that can clamp
    // scrollTop / scrollLeft because the content between spacers is gone.
    var scrollTop  = container.scrollTop;
    var scrollLeft = container.scrollLeft;
    var viewH      = container.clientHeight;

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

    var maxScrollTop = Math.max(0, total * rh - viewH);
    if (scrollTop > maxScrollTop) {
      scrollTop = maxScrollTop;
    }
    var overscan  = this._overscan;
    var startIdx  = Math.max(0, Math.floor(scrollTop / rh) - overscan);
    var endIdx    = Math.min(total, Math.ceil((scrollTop + viewH) / rh) + overscan);

    if (startIdx >= endIdx) {
      startIdx = Math.max(0, endIdx - 1);
    }

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

    // Restore scroll position after DOM mutations.
    // When called from softUpdate (e.g. SSE live updates) we intentionally
    // skip the unconditional set so we do NOT interrupt any in-flight
    // smooth-scroll momentum from a mouse wheel or trackpad.
    // We only clamp scrollTop when the data shrank and the saved position
    // would now be beyond the valid maximum; scrollLeft is not touched
    // because the table column widths do not change between SSE ticks.
    if (this._noScrollRestore) {
      if (container.scrollTop > maxScrollTop) {
        container.scrollTop = maxScrollTop;
      }
    } else {
      container.scrollTop  = scrollTop;
      container.scrollLeft = scrollLeft;
    }

    // Auto-measure actual row height after the first real render.
    // Use a flag so this works regardless of the configured initial rowHeight.
    if (!this._rhMeasured) {
      var first = top.nextSibling;
      if (first && first !== bot) {
        var h = first.getBoundingClientRect().height;
        if (h > 4) {
          this._rh         = h;
          this._rhMeasured = true;
        }
      }
    }

    // Prune cache entries far outside the visible range to bound memory usage.
    // Keep a wide buffer (8x overscan) to avoid recreating rows on fast scrolls.
    var pruneMargin = overscan * 8;
    var keepStart = Math.max(0, startIdx - pruneMargin);
    var keepEnd   = Math.min(total, endIdx + pruneMargin);
    for (var key in cache) {
      if (cache.hasOwnProperty(key)) {
        var idx = parseInt(key, 10);
        if (idx < keepStart || idx >= keepEnd) {
          delete cache[key];
        }
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

  /**
   * Replace the data array without rebuilding spacers or re-binding scroll
   * listeners.  Clears the node cache so all visible rows are recreated on
   * the next render(), but preserves scroll position because the spacer
   * DOM elements and their parent structure are untouched.
   * Ideal for incremental SSE-driven updates where the overall structure
   * is unchanged but row data has been refreshed.
   * @param {Array} rows
   */
  VirtualList.prototype.softUpdate = function (rows) {
    this.rows   = rows || [];
    this._cache = {};
    // Tell render() to preserve the browser's natural scroll position so that
    // in-flight momentum scroll (wheel / touchpad inertia) is not interrupted.
    this._noScrollRestore = true;
    this.render();
    this._noScrollRestore = false;
  };

  /** Disconnect observers. Safe to discard the instance after this. */
  VirtualList.prototype.destroy = function () {
    if (this._ro)          { this._ro.disconnect();          this._ro = null; }
    if (this._containerRO) { this._containerRO.disconnect(); this._containerRO = null; }
    if (this._raf)         { cancelAnimationFrame(this._raf); this._raf = null; }
    this.rows   = [];
    this._cache = {};
  };

  global.VirtualList = VirtualList;
}(window));
