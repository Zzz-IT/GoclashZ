export const normalizeStartupTaskInfo = (info: any) => {
  if (!info) return null;

  return {
    exists: info.exists ?? info.Exists ?? false,
    enabled: info.enabled ?? info.Enabled ?? false,
    mode: info.mode ?? info.Mode ?? 'disabled',
    path: info.path ?? info.Path ?? '',
    arguments: info.arguments ?? info.Arguments ?? '',
    runLevel: info.runLevel ?? info.RunLevel ?? -1,
    lastError: info.lastError ?? info.LastError ?? '',
    expectedPath: info.expectedPath ?? info.ExpectedPath ?? '',
    actualPath: info.actualPath ?? info.ActualPath ?? '',
    actualArgs: info.actualArgs ?? info.ActualArgs ?? '',
    isHealthy: info.isHealthy ?? info.IsHealthy ?? false,
  };
};
