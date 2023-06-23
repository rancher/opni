import Day, { Dayjs } from 'dayjs';

export class FromTo {
    from: Dayjs;
    to: Dayjs;

    constructor(from: any, to: any) {
      this.from = Day(from);
      this.to = Day(to);
    }
}
