import { registerWebModule, NativeModule } from 'expo';

// SpawnFileDigestModule is not available on the web platform.
class SpawnFileDigestModule extends NativeModule {}

export default registerWebModule(SpawnFileDigestModule, 'SpawnFileDigestModule');
