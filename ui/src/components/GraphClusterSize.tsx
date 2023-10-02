import React from "react";
import Snapshot from "../types/Snapshot";
import {Bar, Cell, ComposedChart, ResponsiveContainer, XAxis, YAxis} from "recharts";

const GraphClusterSize = (props: { data: Snapshot[] }) => {
    const {data} = props;

    return (
        <ResponsiveContainer width="100%" height={120}>
            <ComposedChart
                style={{cursor: 'crosshair'}}
                data={data}

            >
                <XAxis dataKey="timestamp" tickFormatter={(timeStr: string) => timeStr.slice(11, 16)}/>
                <YAxis/>
                <Bar dataKey="num_readers"
                     barSize={20}
                     style={{transition: 'fill 0.3s'}}
                >
                    {data.map((snapshot: Snapshot, index) => (
                        <Cell key={index}
                              fill={snapshot.future_value ? 'rgb(171,35,35)' : snapshot.predicted_value ? 'rgb(0,162,191)' : 'rgb(140,140,140)'}
                        />
                    ))}
                </Bar>
            </ComposedChart>
        </ResponsiveContainer>
    )
}
export default GraphClusterSize;