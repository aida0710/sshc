// Claude Code fills the cells under an iTerm2 image with a blank braille
// pattern (U+2800) followed by combining marks that number the image, its row
// and its column. iTerm2 paints the picture over the whole box. The image
// addon keeps the picture's aspect ratio inside the box instead, so the cells
// it leaves uncovered show the marks, which fonts draw as boxes. Dropping the
// marks leaves those cells blank without moving anything: the marks take no
// width of their own.
const placeholderMarks = /⠀\p{M}+/gu;
const leadingMarks = /^\p{M}+/u;
const blankBraille = "⠀";

// createImagePlaceholderMarkFilter returns a function that strips the marks
// from each chunk of output while enabled says so. A chunk can end between
// U+2800 and its marks, so the marks at the start of the next chunk go too.
export function createImagePlaceholderMarkFilter(enabled: () => boolean): (text: string) => string {
  let isAfterPlaceholder = false;
  return (text) => {
    if (!enabled()) {
      isAfterPlaceholder = false;
      return text;
    }
    const continued = isAfterPlaceholder ? text.replace(leadingMarks, "") : text;
    const filtered = continued.replace(placeholderMarks, blankBraille);
    if (filtered !== "") isAfterPlaceholder = filtered.endsWith(blankBraille);
    return filtered;
  };
}
