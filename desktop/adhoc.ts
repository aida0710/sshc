// 束に、少なくとも自分自身と辻褄の合う署名を付ける。
//
// **配布署名の代わりではない。** Gatekeeper は ad-hoc 署名を信頼しないので、
// ダウンロードした人は初回に一度「このまま開く」を通ることになる。ここが直すのは
// その手前の別の問題である。
//
// electron-builder は Electron の実行体を貰ってきて、Info.plist も icon も
// resources も差し替える。**署名はその前のものが残る**ので、封と中身が食い違う。
// 実際 v0.1.0 の arm64 はこうなっていた:
//
//     codesign -dv:   flags=0x20002(adhoc,linker-signed)  Identifier=Electron
//     spctl --assess: code has no resources but signature indicates they must be present
//
// この状態は「開発元を確認できません」を越えたあとに「壊れているため開けません」
// になる。**arm64 では署名の無い実行体はそもそも起動できない**ので、ここは
// 省略できる装飾ではない。x86_64 が動いていたのは、あちらに同じ規則が無いから
// にすぎない。
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import type { AfterPackContext } from "electron-builder";

export default async function signAdHoc(context: AfterPackContext): Promise<void> {
	if (context.electronPlatformName !== "darwin") {
		return;
	}
	// **本物の証明書があるなら、何もしない。** electron-builder はこの後の段で
	// 署名し、環境変数が揃っていれば公証まで通す。ここで先に ad-hoc を被せる
	// 必要も、被せてよい理由も無い。
	//
	// **見るのは identity ではなく、証明書があるかどうかである。** かつては
	// `identity: null`（= ad-hoc せよ）という設定を見ていたが、Developer ID を
	// 使うようになってその設定自体を外した。CSC_LINK があるか、あるいは
	// auto-discovery を切っていないなら、署名は向こうの仕事である。
	const certificate = process.env["CSC_LINK"] ?? process.env["CSC_NAME"] ?? "";
	const discovery = process.env["CSC_IDENTITY_AUTO_DISCOVERY"] ?? "";
	if (certificate !== "" || discovery.toLowerCase() !== "false") {
		return;
	}
	const bundle = join(context.appOutDir, `${context.packager.appInfo.productFilename}.app`);
	// --deep は入れ子のコードも署名する。推奨されない道具だが、代わりに要るのは
	// 「framework と helper を内側から順に」という手順の写しであり、ad-hoc の
	// ためにそれを持つ理由が無い。
	execFileSync("codesign", ["--force", "--deep", "--sign", "-", bundle], { stdio: "inherit" });
};
