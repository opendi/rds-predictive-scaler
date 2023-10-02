import React from 'react';
import { Card, CardContent, Typography } from '@mui/material';
import AnimatedIcon from './AnimatedIcon';
import ScaleStatus from "../types/ScaleStatus";

interface CooldownVisualizerProps {
    scaleInStatus: ScaleStatus|null;
    scaleOutStatus: ScaleStatus|null;
}

const CooldownVisualizer: React.FC<CooldownVisualizerProps> = ({ scaleInStatus, scaleOutStatus }) => {
    return (
        <Card sx={{ width: '30%', mr: 1, display: 'flex', flexWrap: 'wrap', padding: '10px' }}>
            {scaleInStatus &&
            <>
                <Typography variant="h6" gutterBottom>
                    Scaling In
                </Typography>
                <div style={{ display: 'flex', alignItems: 'center' }}>
                    <AnimatedIcon
                        isScalingIn={scaleInStatus.is_scaling}
                        isScalingOut={false}
                        size={40}
                    />
                    <div style={{ marginLeft: '10px' }}>
                        <Typography>
                            Last Scale: {scaleInStatus.last_scale}
                        </Typography>
                        <Typography>
                            Timeout: {scaleInStatus.timeout}
                        </Typography>
                    </div>
                </div>
            </>}

            {scaleOutStatus &&
            <>
                <Typography variant="h6" gutterBottom>
                    Last activity: Scale out
                </Typography>
                <div style={{ display: 'flex', alignItems: 'center' }}>
                    <AnimatedIcon
                        isScalingIn={false}
                        isScalingOut={scaleOutStatus.is_scaling}
                        size={40}
                    />
                    <div style={{ marginLeft: '10px' }}>
                        <Typography>
                            Last Scale: {scaleOutStatus.last_scale}
                        </Typography>
                        <Typography>
                            Timeout: {scaleOutStatus.timeout}
                        </Typography>
                    </div>
                </div>
            </>}
        </Card>
    );
};

export default CooldownVisualizer;
