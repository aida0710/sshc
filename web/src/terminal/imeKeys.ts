export const imeKeyCode = 229;
export function withholdFromTerminal(keyCode: number, composing: boolean): boolean {
  return keyCode === imeKeyCode && !composing;
}

type Attachment = {
  container: HTMLElement;
  textarea: HTMLElement;
};

export function attachImeKeys({ container, textarea }: Attachment): () => void {
  let composing = false;
  const began = () => {
    composing = true;
  };
  const ended = () => {
    composing = false;
  };
  const withhold = (event: KeyboardEvent) => {
    if (withholdFromTerminal(event.keyCode, composing)) event.stopPropagation();
  };

  textarea.addEventListener("compositionstart", began, true);
  textarea.addEventListener("compositionend", ended, true);
  container.addEventListener("keydown", withhold, true);

  return () => {
    textarea.removeEventListener("compositionstart", began, true);
    textarea.removeEventListener("compositionend", ended, true);
    container.removeEventListener("keydown", withhold, true);
  };
}
