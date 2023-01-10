export interface IFeatureFlagProvider {
  areCommunicationsEnabled(): Promise<boolean>;
}
