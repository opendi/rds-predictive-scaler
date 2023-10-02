import Snapshot from "./Snapshot";

interface Broadcast {
    type: string;
    data: Snapshot|Snapshot[]|any
}

export default Broadcast;