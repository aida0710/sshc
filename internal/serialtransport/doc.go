// Package serialtransport は、ローカルのシリアルポートを列挙し、対話的な
// byte stream として開くための境界を提供する。
//
// 接続先の保存やCLIの解釈はこのpackageの責務ではない。Configは接続のたびに
// 渡され、返されたStreamはcontextの取消またはCloseで必ずdeviceを解放する。
package serialtransport
