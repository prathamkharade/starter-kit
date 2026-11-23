import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// true when some network interface is up. (Captive portals can still fail —
/// API errors of type `offline` are the ground truth; this is the trigger to retry.)
final onlineProvider = StreamProvider<bool>((ref) async* {
  final c = Connectivity();
  bool up(List<ConnectivityResult> r) => r.any((x) => x != ConnectivityResult.none);
  yield up(await c.checkConnectivity());
  yield* c.onConnectivityChanged.map(up).distinct();
});
