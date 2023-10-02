import React from "react";
import {Box, LinearProgress, Typography} from "@mui/material";
import Snapshot from "../types/Snapshot";
import {CartesianGrid, ComposedChart, Line, ResponsiveContainer, XAxis, YAxis} from "recharts";

const GraphUtilization = (props: { data: Snapshot[]}) => {
    const {data} = props;

    return (
        <ResponsiveContainer width="100%" height={300}>
            <ComposedChart
                style={{cursor: 'crosshair'}}
                data={data}
            >
                <XAxis dataKey="timestamp" tickFormatter={() => ''}/>
                <YAxis yAxisId="left"/>
                <CartesianGrid strokeDasharray="2 1" opacity={0.3}/>
                <Line
                    type="monotone"
                    dataKey="cluster_utilization"
                    stroke="#82ca9d"
                    strokeWidth={2}
                    dot={false}
                    yAxisId="left"
                ></Line>
                <Line
                    type="monotone"
                    dataKey="max_cpu_utilization"
                    stroke="#fff"
                    strokeWidth={2}
                    dot={false}
                    yAxisId="left"
                />
            </ComposedChart>
        </ResponsiveContainer>
    )
}
export default GraphUtilization;