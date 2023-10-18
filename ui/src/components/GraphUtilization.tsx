import React from 'react';
import {CartesianGrid, ComposedChart, Line, ReferenceLine, ResponsiveContainer, XAxis, YAxis} from 'recharts';
import Snapshot from "../types/Snapshot.ts";
import {Box, Typography} from "@mui/material";
import {MultilineChart} from "@mui/icons-material";

interface GraphUtilizationProps {
    data: Snapshot[];
    targetCpuUtilization?: number;
}

const GraphUtilization: React.FC<GraphUtilizationProps> = ({data, targetCpuUtilization}) => {
    return (
        <Box>
            <Box>
                <Typography gutterBottom variant="h4" component="div">
                    <MultilineChart fontSize={"large"}/> Cluster Chart
                </Typography>
            </Box>

            <Box>
                <ResponsiveContainer width="100%" height={300}>
                    <ComposedChart style={{cursor: 'crosshair'}} data={data}>
                        <XAxis dataKey="timestamp" tickFormatter={() => ''}/>
                        <YAxis yAxisId="left"/>
                        <CartesianGrid strokeDasharray="2 1" opacity={0.3}/>
                        <Line
                            type="monotone"
                            dataKey="max_cpu_utilization"
                            stroke="#fff"
                            strokeWidth={2}
                            dot={false}
                            yAxisId="left"
                        />
                        {targetCpuUtilization !== undefined && (
                            <ReferenceLine yAxisId="left" y={targetCpuUtilization} stroke="red" opacity={0.8}/>
                        )}
                        {/*<ReferenceLine yAxisId="left" y={averageValue} stroke="blue" opacity={0.8} label={`Average (${averageValue.toFixed(2)})`} />*/}
                    </ComposedChart>
                </ResponsiveContainer>
            </Box>
        </Box>
    );
};

export default GraphUtilization;
