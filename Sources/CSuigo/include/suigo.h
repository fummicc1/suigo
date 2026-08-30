#ifndef SUIGO_H
#define SUIGO_H

// suigo の C ABI。境界を跨ぐのはバイトバッファだけである（design §0-1、AR-3 より狭い）。
//
// このヘッダは手書きである。//export 関数を main 以外のパッケージに置くと cgo はヘッダを
// 生成しないため、公開 API の面をここで固定する（Task 1.2 で判明）。
// 実装は embedded/export.go。シグネチャを変えるときは両方を同時に直すこと。

#ifdef __cplusplus
extern "C" {
#endif

/// 登録済みアプリのレンダーループを起動し、ハンドルを返す。失敗時は負値。メインスレッドから呼ぶ。
extern int suigo_mount(void);

/// ループを止める。ブロック中の suigo_next_frame は NULL を返して戻る。メインスレッドから呼ぶ。
extern void suigo_unmount(int handle);

/// ホスト → Go のフレームを渡す。呼び出し中にコピーするので、戻った直後に buf を解放してよい。
/// 成功で 0。メインスレッドから呼び、即座に戻る。
extern int suigo_send(int handle, void *buf, int len);

/// 次のフレームが出るまでブロックし、malloc されたバッファを返す。長さは *outLen に入る。
/// 呼び出し元はコピー後に suigo_free で解放する。unmount 後は NULL（*outLen = 0）。
/// **メインスレッドから呼んではならない。** 専用スレッドで回すこと。
extern void *suigo_next_frame(int handle, int *outLen);

/// suigo_next_frame が返したバッファを解放する。NULL を渡してもよい。
extern void suigo_free(void *p);

#ifdef __cplusplus
}
#endif

#endif /* SUIGO_H */
