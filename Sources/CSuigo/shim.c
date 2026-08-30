// CSuigo は宣言だけを提供する C ターゲット。SwiftPM は空のターゲットを許さないため、
// この翻訳単位を置く。実体は Go の c-archive（libsuigo.a）にあり、アプリ側でリンクする。
#include "suigo.h"
