(() => {
  const viewport = { width: innerWidth, height: innerHeight, dpr: devicePixelRatio };
  const elements = [];
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
  let visited = 0;
  while (walker.nextNode()) {
    if (++visited > 5000) return { ...viewport, elements: [], truncated: true };
    const node = walker.currentNode;
    const id = node.getAttribute("data-testid");
    if (!id) continue;
    const rect = node.getBoundingClientRect();
    const style = getComputedStyle(node);
    elements.push({
      id,
      parentId: node.parentElement.getAttribute("data-testid") || "",
      visible: node.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })
        && rect.width > 0 && rect.height > 0,
      x: rect.x, y: rect.y, width: rect.width, height: rect.height,
      borderTop: parseFloat(style.borderTopWidth),
      paddingTop: parseFloat(style.paddingTop),
      geometrySupported: (() => {
        if (style.position === "absolute" || style.position === "fixed") return false;
        for (let ancestor = node; ancestor; ancestor = ancestor.parentElement) {
          const computed = getComputedStyle(ancestor);
          if (computed.transform !== "none" || computed.translate !== "none" ||
              computed.rotate !== "none" || computed.scale !== "none" ||
              computed.zoom !== "1" || ancestor.scrollTop !== 0) return false;
        }
        return true;
      })(),
    });
  }
  return { ...viewport, elements };
})()
