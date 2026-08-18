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
const { execFileSync } = require("node:child_process");
const { join } = require("node:path");

exports.default = async function signAdHoc(context) {
	if (context.electronPlatformName !== "darwin") {
		return;
	}
	// **本物の証明書があるなら、何もしない。** electron-builder はこの後の段で
	// 署名する。ここで先に ad-hoc を被せる必要も、被せてよい理由も無い。
	if (context.packager.platformSpecificBuildOptions.identity !== null) {
		return;
	}
	const bundle = join(context.appOutDir, `${context.packager.appInfo.productFilename}.app`);
	// --deep は入れ子のコードも署名する。推奨されない道具だが、代わりに要るのは
	// 「framework と helper を内側から順に」という手順の写しであり、ad-hoc の
	// ためにそれを持つ理由が無い。
	execFileSync("codesign", ["--force", "--deep", "--sign", "-", bundle], { stdio: "inherit" });
};
