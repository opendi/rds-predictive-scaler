export interface Config {
    MaxInstances: number;
    MinInstances: number;
    TargetCpuUtil: number;
    PlanAheadTime: number;
    ScaleOutCooldown: number;
    ScaleInCooldown: number;
    ScaleInStep: number;
    ScaleOutStep: number;
}
