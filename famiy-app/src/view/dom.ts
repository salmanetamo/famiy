/** Marks the toolbar and the selection summary, which sit on top of the pannable graph. */
export const GRAPH_CONTROLS_ATTRIBUTE = 'data-graph-controls';

/**
 * True when an event started on one of the graph's own controls. Those events bubble through the
 * pannable viewport, so without this check pressing "Center" would also count as a click on the
 * background (clearing the selection) and a press on "+" could start a drag.
 */
export function isFromGraphControl(target: EventTarget): boolean {
  return target instanceof Element && target.closest(`[${GRAPH_CONTROLS_ATTRIBUTE}]`) !== null;
}
