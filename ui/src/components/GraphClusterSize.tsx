import {Bar, ComposedChart, ResponsiveContainer, XAxis, YAxis} from "recharts";

const GraphClusterSize = (props: { data: ClusterStatus[] }) => {
    const {data} = props;

    return (
        <ResponsiveContainer width="100%" height={120}>
            <ComposedChart
                style={{cursor: 'crosshair'}}
                data={data}
            >
                <XAxis dataKey="timestamp" tickFormatter={(timeStr: string) => timeStr.slice(11, 16)}/>
                <YAxis tickCount={3} interval={1}/>
                <Bar dataKey="current_active_readers" barSize={20}  />
                <Bar dataKey="optimal_size" barSize={20}  />
            </ComposedChart>
        </ResponsiveContainer>
    )
}

export default GraphClusterSize;
