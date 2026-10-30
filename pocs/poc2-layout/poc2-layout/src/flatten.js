'use strict';

/**
 * Flatten a (possibly hierarchical) laid-out ELK graph into absolute-positioned
 * leaf nodes. ELK reports a child's x/y relative to its parent container, so we
 * accumulate offsets down the tree. Containers are included too (flagged), so a
 * renderer can draw them faintly; metrics code filters them out.
 */
function flattenNodes(laid) {
  const out = [];
  const walk = (node, ox, oy) => {
    for (const c of node.children || []) {
      const ax = ox + c.x;
      const ay = oy + c.y;
      const isContainer = !!(c.children && c.children.length);
      out.push({ id: c.id, x: ax, y: ay, width: c.width, height: c.height, isContainer, raw: c });
      if (isContainer) walk(c, ax, ay);
    }
  };
  walk(laid, 0, 0);
  return out;
}

module.exports = { flattenNodes };
