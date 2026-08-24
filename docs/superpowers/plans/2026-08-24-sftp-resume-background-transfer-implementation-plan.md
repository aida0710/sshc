# SFTP Resumable Background Transfer Implementation Plan

**Goal:** 2 MiBを超えるファイルと複数ファイルを安全に転送し、SFTP画面を離れても処理を継続できるようにする。通信断時は完了済みoffsetから再開し、未完成データを完成ファイルとして見せない。

**Architecture:** uploadはWebの転送queueが1 MiB chunkへ分割し、Go側のresumable upload APIが対象と同じremote directoryに予約名のpart fileを保持する。各chunkは期待offsetと一致する場合だけ追記し、完了時にtarget revisionを再検証してatomic renameする。queueはSFTP React component外のsingleton storeに置き、route移動後も継続する。file downloadはHTTP Rangeと同じqueueで再試行し、folder ZIPは生成streamの再現性を保証できないためresume対象外とする。

**Tech Stack:** Go / Echo / github.com/pkg/sftp / OpenAPI / React / TypeScript / Vitest / Playwright / OpenSSH container integration

## Constraints

- remote targetは完了時まで変更せず、part fileから同一directory内のrenameで公開する。
- targetがinit後に変更された場合、overwrite指定済みでもrevision不一致として完了を拒否する。
- chunkは既存のAPI request body上限以下に収め、巨大なrequestを許可しない。
- transfer ID、remote path、offset、sizeを検証し、offsetの飛び越し・巻き戻し・別pathへの流用を拒否する。
- pauseはpart fileを維持し、cancelはpart fileを削除する。
- route移動中の継続を「background」とする。browserを閉じた後の自動継続やengine再起動中の処理継続は保証しない。
- browser reload後はFile objectを復元できないため、同じname・size・lastModifiedのfileを再選択した場合だけuploadを再開する。
- file downloadはRangeで再開する。directory ZIP downloadはresume対象外。
- SFTP favoriteは追加しない。
- `web/src`変更後は`internal/ui/dist`をproduction buildで更新する。

## Acceptance Criteria

- [x] 2 MiBを超えるfileを1 MiB以下のchunkでuploadできる。
- [x] init APIは既存part fileのoffsetを返し、同じtransfer IDとfileを再接続すると続きから送る。
- [x] 不正offset、size超過、target revision競合を拒否する。
- [x] pause後はpart fileを残して再開し、cancel後はpart fileを削除する。
- [x] upload完了前にtargetは作成・置換されず、完了時だけatomic renameされる。
- [x] 複数fileを上限付き並列queueで処理し、1件の失敗後も他jobを継続する。
- [x] SFTP画面から別画面へ移動して戻ってもjobとprogressが残り、転送が継続する。
- [x] reload後に中断jobを表示し、同一fileの再選択でremote offsetから再開する。
- [x] file downloadはRange requestで中断offsetから再試行できる。
- [x] UIに待機・転送中・一時停止・再開待ち・完了・失敗・cancel状態とbytes進捗を表示する。
- [x] Go unit/API、Web unit/typecheck/lint、OpenAPI生成、production build、OpenSSH container integrationが成功する。
- [x] desktopとmobile幅の実画面をscreenshotで確認する。

## Phases

### Phase 1: Resumable upload backend

- resumable upload sessionの型、ID/path/offset検証、予約part pathを追加する。
- init、chunk append、complete、cancelを直列化するtransfer managerを実装する。
- `Remote`へseek可能なopenを追加し、pkg/sftp adapterとfakeを更新する。
- HTTP routes、problem mapping、OpenAPI schema、API testを追加する。

### Phase 2: Background queue and resume UX

- component外にupload/download job store、subscriber、bounded worker queueを追加する。
- File metadataだけをlocalStorageへ保存し、reload後は`reattach required`として復元する。
- SFTPPanelの逐次upload stateをqueue表示へ置換し、pause/resume/cancel/reselectを追加する。
- overwrite確認はinit競合jobに対して行い、明示選択後だけoverwrite sessionを開始する。
- file downloadにRange再試行を追加し、folder ZIPは既存経路を維持する。

### Phase 3: Verification and release

- service、HTTP、store、componentの境界testを追加する。
- fixed OpenSSH containerでlarge upload、pause/resume、atomic complete、cancel cleanup、download rangeを確認する。
- browser E2Eとdesktop/mobile screenshotでlayout・progress・操作可視性を確認する。
- 全test、race、typecheck、lint、buildを通し、v0.8.1をcommit・push・releaseする。

## Risks

- browserのBlob downloadは全内容をmemoryに保持する。v0.8.1では既存制約を維持し、File System Access APIによるdisk直書きは別改善とする。
- remote serverがrename置換拡張を持たない場合は既存のSFTP rename semanticsに従う。競合検査と同一directory part fileで危険を限定する。
- browser reload後のFile再選択は利用者操作が必要。File objectを永続化せず、名前・size・更新時刻が一致しないfileは再開に使わない。
- 複数jobが同じtargetを狙う場合はcomplete時のrevision検査で片方を拒否し、暗黙のlast-write-winsにしない。
