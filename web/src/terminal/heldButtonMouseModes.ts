import type { IModes, Terminal } from "@xterm/xterm";

// マウスのボタンを押している間に届いた、マウスの報告を減らすモードの指定を、ボタンを離すまで待たせる。
//
// Claude Code は、入力が数秒とぎれたあとの最初の入力で、マウスのモードを ?1000h、?1002h、?1003h の順に
// 送り直す。なぞって選んでいる途中で手を止めて読み、続きをなぞると、ボタンを押したままの間に届く。モードは
// あとに届いたものが効くので、送り直しのあとは元と同じ 1003 に戻る。ところが xterm.js は、?1000h で
// 押したままの移動（ドラッグ）の報告をいったんやめ、その報告を次にボタンが押されるまで再開しない。Claude Code
// は離した位置ではなく最後に届いた移動までを選ぶので、手を止めた所までしかコピーされなかった。
//
// 報告を減らす指定を離すまで待たせれば、押している間に報告が止まらない。送り直しでは、元のモードに戻す指定が
// 続いて届くので、待たせた指定は捨てる。本当に減らす指定だったときは、離したところで効かせる。

type TrackingMode = IModes["mouseTrackingMode"];

// 報告するイベントの少ない順。あとのモードは、前のモードが報告するイベントをすべて報告する。
const reportedEventsOrder: readonly TrackingMode[] = ["none", "x10", "vt200", "drag", "any"];
// マウスのモードを指定する DECSET の番号。
const trackingModeOfParameter: ReadonlyMap<number, TrackingMode> = new Map([[9, "x10"], [1000, "vt200"], [1002, "drag"], [1003, "any"]]);

type MouseModeTerminal = Pick<Terminal, "parser" | "write"> & { readonly modes: Pick<IModes, "mouseTrackingMode"> };

export function attachHeldButtonMouseModes(view: MouseModeTerminal, container: HTMLElement): () => void {
  const page = container.ownerDocument;
  let buttonHeld = false;
  // 待たせている指定の番号。
  let waiting: number | null = null;

  const released = () => {
    if (!buttonHeld) return;
    buttonHeld = false;
    page.removeEventListener("mouseup", buttonChanged, true);
    page.removeEventListener("mousemove", buttonChanged, true);
    page.defaultView?.removeEventListener("blur", released);
    if (waiting === null) return;
    view.write(`\u001b[?${waiting}h`);
    waiting = null;
  };
  // ボタンがどれも押されていなければ、離したと読む。ウィンドウの外で離して mouseup が届かなかったときは、
  // 次の移動で分かる。
  function buttonChanged(event: MouseEvent) {
    if (event.buttons === 0) released();
  }
  const pressed = () => {
    if (buttonHeld) return;
    buttonHeld = true;
    page.addEventListener("mouseup", buttonChanged, true);
    page.addEventListener("mousemove", buttonChanged, true);
    page.defaultView?.addEventListener("blur", released);
  };
  container.addEventListener("mousedown", pressed, true);

  const registrations = [
    view.parser.registerCsiHandler({ prefix: "?", final: "h" }, (params) => {
      // ほかのモードと一緒に指定した列（?1000;1006h）は、一部だけを待たせられないので、そのまま通す。
      const requested = params.length === 1 && typeof params[0] === "number" ? trackingModeOfParameter.get(params[0]) : undefined;
      if (requested === undefined) return false;
      const reportsFewerEvents = reportedEventsOrder.indexOf(requested) < reportedEventsOrder.indexOf(view.modes.mouseTrackingMode);
      if (!buttonHeld || !reportsFewerEvents) {
        waiting = null;
        return false;
      }
      waiting = params[0] as number;
      return true;
    }),
    // マウスの報告をやめる指定は、待たせた指定より新しい。
    view.parser.registerCsiHandler({ prefix: "?", final: "l" }, (params) => {
      if (params.some((parameter) => typeof parameter === "number" && trackingModeOfParameter.has(parameter))) waiting = null;
      return false;
    }),
  ];

  return () => {
    container.removeEventListener("mousedown", pressed, true);
    waiting = null;
    released();
    for (const registration of registrations) registration.dispose();
  };
}
