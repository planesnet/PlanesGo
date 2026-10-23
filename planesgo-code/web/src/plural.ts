// plural: «1 fichero», «3 ficheros». Sin la forma plural, la del singular con «s».
export const plural = (n: number, singular: string, pluralForm = singular + 's') => `${n} ${n === 1 ? singular : pluralForm}`;
