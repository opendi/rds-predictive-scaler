import Snapshot from "../types/Snapshot";
import { Bar, Cell, ComposedChart, ResponsiveContainer, XAxis, YAxis } from "recharts";
import {red} from "@mui/material/colors";

const GraphClusterSize = (props: { data: Snapshot[] }) => {
    const { data } = props;

    // Create a new data array that includes a fixed value of 1 for the red bar
    const clusterData = data.map(snapshot => ({
        ...snapshot,
        fixedValue: 1, // Add a fixed value of 1 for the red bar
    }));

    return (
        <ResponsiveContainer width="100%" height={120}>
            <ComposedChart
                style={{ cursor: 'crosshair' }}
                data={clusterData}
            >
                <XAxis dataKey="timestamp" tickFormatter={(timeStr: string) => timeStr.slice(11, 16)} />
                <YAxis tickCount={3} interval={1} />
                <Bar dataKey="fixedValue" barSize={20} stackId="stack" fill={red[50]} />
                <Bar dataKey="num_readers" barSize={20} stackId="stack">
                    {clusterData.map((snapshot: Snapshot, index) => (
                        <Cell
                            key={index}
                            fill={snapshot.future_value ? 'rgb(171,35,35)' : snapshot.predicted_value ? 'rgb(0,162,191)' : 'rgb(140,140,140)'}
                        />
                    ))}
                </Bar>
            </ComposedChart>
        </ResponsiveContainer>
    )
}

export default GraphClusterSize;
