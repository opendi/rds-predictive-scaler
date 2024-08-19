import {CartesianGrid, ComposedChart, Line, ReferenceLine, ResponsiveContainer, Tooltip, XAxis,} from 'recharts';
import {Box, Typography} from "@mui/material";
import {MultilineChart} from "@mui/icons-material";
import React from "react";

interface GraphUtilizationProps {
    historyData: ClusterStatus[];
    predictionData: ClusterStatus[];
    targetCpuUtilization?: number;
}

const GraphUtilization: React.FC<GraphUtilizationProps> = ({historyData, predictionData, targetCpuUtilization}) => {
    return (
        <Box>
            <Box>
                <Typography gutterBottom variant="h4" component="div">
                    <MultilineChart fontSize={"large"}/> Cluster Chart
                </Typography>
            </Box>

            <Box>
                <ResponsiveContainer width="100%" height={300}>
                    <ComposedChart style={{cursor: 'crosshair'}}>
                        <XAxis
                            dataKey="timestamp"
                            tickFormatter={(timestamp) => {
                                const date = new Date(timestamp);
                                return date.getHours().toString().padStart(2, '0');
                            }}
                            interval={12}
                        />
                        <CartesianGrid strokeDasharray="1 4" opacity={0.3}/>

                        <Line
                            type="monotone"
                            dataKey="average_cpu_utilization"
                            stroke="#ff0000"
                            strokeWidth={2}
                            dot={false}
                            data={predictionData}
                        />

                        <Line
                            type="monotone"
                            dataKey="average_cpu_utilization"
                            stroke="#fff"
                            strokeWidth={2}
                            dot={false}
                            data={historyData}
                        />

                        {targetCpuUtilization !== undefined && (
                            <ReferenceLine y={targetCpuUtilization} stroke="red" opacity={0.8}/>
                        )}

                        <Tooltip
                            position={{x: 0, y: 0}}
                            cursor={{stroke: '#aaa', strokeWidth: 1}}
                            contentStyle={{backgroundColor: '#222', border: 'none'}}
                            formatter={(value: number, name) => {
                                if (name === 'average_cpu_utilization') {
                                    return [`${value.toFixed(2)}%`, 'CPU Utilization'];
                                }
                                if (name === 'optimal_size') {
                                    return [value, 'Optimal cluster size'];
                                }
                                return value;
                            }}
                        />
                    </ComposedChart>
                </ResponsiveContainer>
            </Box>
        </Box>
    );
};

export default GraphUtilization;
